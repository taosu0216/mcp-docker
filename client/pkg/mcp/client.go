package mcp

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ClientManager 管理MCP客户端连接的结构体
type ClientManager struct {
	client      *client.SSEMCPClient
	serverURL   string
	mutex       sync.Mutex
	isConnected bool
	lastError   error
	reconnect   chan struct{} // 用于触发重连的通道
}

// NewClientManager 创建新的MCP客户端管理器
func NewClientManager(serverURL string) *ClientManager {
	return &ClientManager{
		serverURL:   serverURL,
		reconnect:   make(chan struct{}, 1),
		isConnected: false,
	}
}

// GetClient 获取客户端，如果连接异常则尝试重新连接
func (m *ClientManager) GetClient(ctx context.Context) (*client.SSEMCPClient, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 如果客户端尚未创建或连接异常，尝试重新连接
	if m.client == nil || !m.isConnected {
		if err := m.connect(ctx); err != nil {
			return nil, err
		}
	}

	return m.client, nil
}

// 连接到MCP服务器
func (m *ClientManager) connect(ctx context.Context) error {
	// 如果之前有客户端，先关闭它并清空引用
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}

	// 准备服务器URL
	serverURL := m.serverURL
	fmt.Printf("连接到服务器: %s\n", serverURL)

	// 创建新的客户端
	var err error
	m.client, err = client.NewSSEMCPClient(serverURL)
	if err != nil {
		m.isConnected = false
		m.lastError = err
		return fmt.Errorf("创建MCP客户端失败: %v", err)
	}

	// 增强的重试逻辑
	var startErr error
	for retries := 0; retries < 5; retries++ {
		startErrorChannel := make(chan error, 1)

		// 使用goroutine进行连接，避免卡住
		go func() {
			fmt.Printf("尝试启动MCP客户端 (%d/5)...\n", retries+1)
			startErrorChannel <- m.client.Start(ctx)
		}()

		// 设置连接超时
		select {
		case startErr = <-startErrorChannel:
			if startErr == nil {
				fmt.Println("MCP客户端连接成功!")
				break
			}
			fmt.Printf("连接MCP服务器失败: %v，重试中...\n", startErr)
		case <-time.After(5 * time.Second):
			fmt.Println("连接MCP服务器超时，重试中...")
			startErr = fmt.Errorf("连接超时")
		}

		if startErr == nil {
			break
		}

		// 如果连接失败，关闭当前客户端并创建新的客户端
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}

		// 创建新的客户端
		m.client, err = client.NewSSEMCPClient(serverURL)
		if err != nil {
			m.isConnected = false
			m.lastError = err
			return fmt.Errorf("创建MCP客户端失败: %v", err)
		}

		time.Sleep(2 * time.Second)
	}

	if startErr != nil {
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		m.isConnected = false
		m.lastError = startErr
		return fmt.Errorf("无法连接到MCP服务器: %v", startErr)
	}

	// 初始化客户端
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "docker-cli",
		Version: "1.0.0",
	}

	// 初始化请求使用一个单独的超时上下文
	initCtx, initCancel := context.WithTimeout(ctx, 10*time.Second)
	defer initCancel()

	_, err = m.client.Initialize(initCtx, initRequest)
	if err != nil {
		// 初始化失败，关闭客户端
		if m.client != nil {
			m.client.Close()
			m.client = nil
		}
		m.isConnected = false
		m.lastError = err
		return fmt.Errorf("初始化MCP客户端失败: %v", err)
	}

	m.isConnected = true
	m.lastError = nil
	return nil
}

// MarkConnectionFailed 标记连接为失败状态
func (m *ClientManager) MarkConnectionFailed(err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 记录错误并关闭客户端
	m.isConnected = false
	m.lastError = err

	// 关闭客户端以确保重新建立连接
	if m.client != nil {
		m.client.Close()
		m.client = nil
	}

	fmt.Printf("MCP连接已标记为失败状态: %v\n", err)

	// 触发重连信号
	select {
	case m.reconnect <- struct{}{}:
		// 成功发送重连信号
	default:
		// 通道已满，忽略
	}
}

// GetReconnectChannel 获取重连通道
func (m *ClientManager) GetReconnectChannel() <-chan struct{} {
	return m.reconnect
}

// NeedsReconnect 检查客户端是否需要重新连接
func (m *ClientManager) NeedsReconnect() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return !m.isConnected || m.client == nil
}

// APIKeyTransport 是一个自定义的HTTP Transport，用于在每个请求中添加API密钥
type APIKeyTransport struct {
	apiKey string
	base   http.RoundTripper
}

// RoundTrip 实现http.RoundTripper接口
func (t *APIKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 如果设置了API密钥，则添加到请求头
	if t.apiKey != "" {
		// 添加Bearer令牌到Authorization头
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
		// 添加X-API-Key头
		req.Header.Set("X-API-Key", t.apiKey)

		// 同时在URL中添加API密钥作为查询参数（兼容性考虑）
		query := req.URL.Query()
		query.Set("api_key", t.apiKey)
		req.URL.RawQuery = query.Encode()
	}

	// 使用基础Transport处理请求
	return t.base.RoundTrip(req)
}

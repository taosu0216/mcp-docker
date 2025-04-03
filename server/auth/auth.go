package auth

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/server"
)

// DEBUG 标志，控制是否打印详细的调试信息
var DEBUG = true

// 默认的API密钥环境变量名
const DefaultAPIKeyEnvVar = "API_KEY"

// 默认的身份验证配置
const (
	DefaultAPIKeyHeader = "X-API-Key"
	DefaultAPIKeyParam  = "api_key"
)

// 打印详细的请求信息，用于调试
func printRequestDebug(r *http.Request, prefix string) {
	if !DEBUG {
		return
	}

	fmt.Printf("\n%s === 请求详情开始 ===\n", prefix)
	fmt.Printf("%s 方法: %s\n", prefix, r.Method)
	fmt.Printf("%s 路径: %s\n", prefix, r.URL.Path)
	fmt.Printf("%s 查询: %s\n", prefix, r.URL.RawQuery)
	fmt.Printf("%s 远程地址: %s\n", prefix, r.RemoteAddr)
	fmt.Printf("%s User-Agent: %s\n", prefix, r.UserAgent())

	fmt.Printf("%s --- 请求头 ---\n", prefix)
	for name, values := range r.Header {
		fmt.Printf("%s %s: %s\n", prefix, name, strings.Join(values, ", "))
	}

	fmt.Printf("%s --- 查询参数 ---\n", prefix)
	for key, values := range r.URL.Query() {
		fmt.Printf("%s %s: %s\n", prefix, key, strings.Join(values, ", "))
	}

	fmt.Printf("%s === 请求详情结束 ===\n\n", prefix)
}

// MCPAuthenticator 实现MCP鉴权
type MCPAuthenticator struct {
	apiKey     string
	headerName string
	queryParam string
}

// NewMCPAuthenticator 创建一个新的MCP鉴权器
func NewMCPAuthenticator(apiKey, headerName, queryParam string) *MCPAuthenticator {
	if headerName == "" {
		headerName = DefaultAPIKeyHeader
	}
	if queryParam == "" {
		queryParam = DefaultAPIKeyParam
	}

	return &MCPAuthenticator{
		apiKey:     apiKey,
		headerName: headerName,
		queryParam: queryParam,
	}
}

// NewMCPAuthenticatorFromEnv 从环境变量中创建MCP鉴权器
func NewMCPAuthenticatorFromEnv(envVar string) *MCPAuthenticator {
	if envVar == "" {
		envVar = DefaultAPIKeyEnvVar
	}

	apiKey := os.Getenv(envVar)
	return NewMCPAuthenticator(apiKey, DefaultAPIKeyHeader, DefaultAPIKeyParam)
}

// IsConfigured 检查是否配置了API密钥
func (a *MCPAuthenticator) IsConfigured() bool {
	return a.apiKey != ""
}

// String 输出鉴权配置信息
func (a *MCPAuthenticator) String() string {
	isConfigured := a.IsConfigured()
	apiKeyStatus := "未配置"
	if isConfigured {
		apiKeyStatus = "已配置"
	}

	return fmt.Sprintf("MCP API密钥认证: %s", apiKeyStatus)
}

// AuthenticatedMCPServer 为MCP服务器添加鉴权
type AuthenticatedMCPServer struct {
	mcpServer     *server.MCPServer
	authenticator *MCPAuthenticator
	handler       http.Handler
	// 存储已认证的连接会话
	authenticatedSessions map[string]bool
	// 保护会话映射的互斥锁
	sessionMutex sync.RWMutex
}

// NewAuthenticatedMCPServer 创建一个带有鉴权的MCP服务器
func NewAuthenticatedMCPServer(mcpServer *server.MCPServer, authenticator *MCPAuthenticator) *AuthenticatedMCPServer {
	// 当前MCP-Go框架不直接支持拦截初始化请求，因此我们通过HTTP层实现鉴权
	// 后续可以考虑直接修改MCP-Go框架，添加鉴权中间件
	sseServer := server.NewSSEServer(mcpServer)
	return &AuthenticatedMCPServer{
		mcpServer:             mcpServer,
		authenticator:         authenticator,
		handler:               sseServer,
		authenticatedSessions: make(map[string]bool),
	}
}

// NewAuthenticatedMCPServerWithAPIKey 使用指定的API密钥创建带鉴权的MCP服务器
func NewAuthenticatedMCPServerWithAPIKey(mcpServer *server.MCPServer, apiKey string) *AuthenticatedMCPServer {
	authenticator := NewMCPAuthenticator(apiKey, DefaultAPIKeyHeader, DefaultAPIKeyParam)
	return NewAuthenticatedMCPServer(mcpServer, authenticator)
}

// Start 启动MCP服务器，添加鉴权处理
func (s *AuthenticatedMCPServer) Start(address string) error {
	// 如果没有配置API密钥，则记录警告
	if !s.authenticator.IsConfigured() {
		fmt.Println("警告: 未配置API密钥，允许所有请求访问")
	}

	// 创建一个包装的HTTP处理器，用于添加鉴权
	authHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		printRequestDebug(r, "[AUTH]")

		// 添加CORS支持
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		// 处理OPTIONS请求（预检请求）
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 为SSE连接生成唯一会话ID
		sessionID := r.RemoteAddr + "-" + r.Header.Get("User-Agent")

		// 检查此会话是否已认证
		s.sessionMutex.RLock()
		authenticated, ok := s.authenticatedSessions[sessionID]
		s.sessionMutex.RUnlock()

		if ok && authenticated {
			fmt.Println("会话已认证，允许访问")
			s.handler.ServeHTTP(w, r)
			return
		}

		// 如果未配置API密钥，直接放行
		if !s.authenticator.IsConfigured() {
			s.handler.ServeHTTP(w, r)
			return
		}

		// 从请求中获取API密钥
		var authToken string

		// 1. 尝试从Authorization头获取
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			// 检查并移除可能的Bearer前缀
			const bearerPrefix = "Bearer "
			if len(authHeader) > len(bearerPrefix) && strings.HasPrefix(authHeader, bearerPrefix) {
				authToken = authHeader[len(bearerPrefix):]
			} else {
				authToken = authHeader
			}
		}

		// 2. 尝试从X-API-Key头获取
		if authToken == "" {
			authToken = r.Header.Get(s.authenticator.headerName)
		}

		// 3. 尝试从查询参数获取
		if authToken == "" {
			authToken = r.URL.Query().Get(s.authenticator.queryParam)
		}

		// 4. 尝试从Cursor MCP环境配置中获取密钥
		if authToken == "" && strings.Contains(r.UserAgent(), "node") &&
			(r.URL.Path == "/sse" || strings.HasSuffix(r.URL.Path, "/sse")) {
			// 针对Cursor SSE连接的特殊处理 - 临时放行
			fmt.Println("检测到Cursor SSE连接请求，使用配置的API密钥")
			authToken = s.authenticator.apiKey
		}

		// 验证API密钥
		if authToken == "" || authToken != s.authenticator.apiKey {
			fmt.Printf("鉴权失败: 无效的API密钥，收到的密钥: %s\n", authToken)
			http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
			return
		}

		// 认证成功，记录此会话
		s.sessionMutex.Lock()
		s.authenticatedSessions[sessionID] = true
		s.sessionMutex.Unlock()

		fmt.Println("API密钥验证成功，会话已认证")
		s.handler.ServeHTTP(w, r)
	})

	// 创建特殊的无鉴权处理器，专门用于Cursor
	cursorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		printRequestDebug(r, "[CURSOR-HANDLER]")

		// 添加CORS支持
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		// 处理OPTIONS请求（预检请求）
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 只允许来自Cursor的请求
		if !strings.Contains(r.UserAgent(), "node") {
			fmt.Println("非Cursor请求尝试访问无鉴权端点，拒绝访问")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		fmt.Println("Cursor请求通过无鉴权端点连接，允许访问")

		// 自动添加此会话到已认证列表
		sessionID := r.RemoteAddr + "-" + r.Header.Get("User-Agent")
		s.sessionMutex.Lock()
		s.authenticatedSessions[sessionID] = true
		s.sessionMutex.Unlock()

		s.handler.ServeHTTP(w, r)
	})

	// 添加一个健康检查端点
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 确定URL路径
	urlPath := "/"
	hostPort := address

	if strings.Contains(address, "/") {
		parts := strings.SplitN(address, "/", 2)
		if len(parts) == 2 {
			hostPort = parts[0]
			urlPath = "/" + parts[1]
		}
	}

	// 设置路由
	mux := http.NewServeMux()
	mux.Handle(urlPath, RequestBodyLoggingHandler(authHandler))

	// 添加专门的SSE端点，用于Cursor MCP连接
	mux.Handle("/sse", RequestBodyLoggingHandler(authHandler))

	// 添加一个无鉴权的SSE端点，专门给Cursor使用
	mux.Handle("/cursor-sse", RequestBodyLoggingHandler(cursorHandler))

	// 添加一个健康检查端点
	mux.Handle("/health", RequestBodyLoggingHandler(healthHandler))

	// 启动HTTP服务器
	fmt.Printf("启动MCP SSE服务器，监听地址: %s，路径: %s\n", hostPort, urlPath)
	fmt.Printf("额外SSE端点: http://%s/sse\n", hostPort)
	fmt.Printf("Cursor专用无鉴权端点: http://%s/cursor-sse\n", hostPort)
	fmt.Printf("鉴权配置: %s\n", s.authenticator.String())
	fmt.Println("请在Cursor MCP配置中使用URL: http://localhost:12345/cursor-sse")
	return http.ListenAndServe(hostPort, mux)
}

// ServeHTTP 实现http.Handler接口
func (s *AuthenticatedMCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	printRequestDebug(r, "[ServeHTTP]")

	// 添加CORS支持
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

	// 处理OPTIONS请求（预检请求）
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 为SSE连接生成唯一会话ID
	sessionID := r.RemoteAddr + "-" + r.Header.Get("User-Agent")

	// 检查此会话是否已认证
	s.sessionMutex.RLock()
	authenticated, ok := s.authenticatedSessions[sessionID]
	s.sessionMutex.RUnlock()

	if ok && authenticated {
		s.handler.ServeHTTP(w, r)
		return
	}

	// 如果未配置API密钥，直接放行
	if !s.authenticator.IsConfigured() {
		s.handler.ServeHTTP(w, r)
		return
	}

	// 从请求中获取API密钥
	var authToken string

	// 1. 尝试从Authorization头获取
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		// 检查并移除可能的Bearer前缀
		const bearerPrefix = "Bearer "
		if len(authHeader) > len(bearerPrefix) && strings.HasPrefix(authHeader, bearerPrefix) {
			authToken = authHeader[len(bearerPrefix):]
		} else {
			authToken = authHeader
		}
	}

	// 2. 尝试从X-API-Key头获取
	if authToken == "" {
		authToken = r.Header.Get(s.authenticator.headerName)
	}

	// 3. 尝试从查询参数获取
	if authToken == "" {
		authToken = r.URL.Query().Get(s.authenticator.queryParam)
	}

	// 4. 尝试从Cursor MCP环境配置中获取密钥
	if authToken == "" && strings.Contains(r.UserAgent(), "node") &&
		(r.URL.Path == "/sse" || strings.HasSuffix(r.URL.Path, "/sse")) {
		// 针对Cursor SSE连接的特殊处理 - 临时放行
		fmt.Println("检测到Cursor SSE连接请求，使用配置的API密钥")
		authToken = s.authenticator.apiKey
	}

	// 验证API密钥
	if authToken == "" || authToken != s.authenticator.apiKey {
		fmt.Printf("ServeHTTP鉴权失败: 无效的API密钥，收到的密钥: %s\n", authToken)
		http.Error(w, "Unauthorized: invalid API key", http.StatusUnauthorized)
		return
	}

	// 认证成功，记录此会话
	s.sessionMutex.Lock()
	s.authenticatedSessions[sessionID] = true
	s.sessionMutex.Unlock()

	fmt.Println("API密钥验证成功，会话已认证")
	s.handler.ServeHTTP(w, r)
}

// 验证MCP请求的会话
// 注意：此方法是为未来扩展准备的，当前版本不会被调用
func (s *AuthenticatedMCPServer) ValidateSession(sessionID string) bool {
	// 如果未配置API密钥，总是返回认证成功
	if !s.authenticator.IsConfigured() {
		return true
	}

	// 检查会话是否已认证
	s.sessionMutex.RLock()
	authenticated, ok := s.authenticatedSessions[sessionID]
	s.sessionMutex.RUnlock()

	return ok && authenticated
}

// 清理过期会话
// 注意：此方法是为未来扩展准备的，当前版本不会被调用
func (s *AuthenticatedMCPServer) CleanupSessions(expiredSessions []string) {
	s.sessionMutex.Lock()
	defer s.sessionMutex.Unlock()

	for _, sessionID := range expiredSessions {
		delete(s.authenticatedSessions, sessionID)
	}
}

// PrintCursorMCPGuide 打印Cursor MCP配置指南
func PrintCursorMCPGuide(apiKey string) {
	fmt.Println("\n=== Cursor MCP配置指南 ===")
	fmt.Println("1. 打开Cursor设置 (Ctrl+,)")
	fmt.Println("2. 选择左侧的MCP选项")
	fmt.Println("3. 点击 '添加新的全局MCP服务器' 按钮")
	fmt.Println("4. 使用以下配置:")
	fmt.Println("   - 服务器名称: server-name")
	fmt.Println("   - URL: http://localhost:12345/cursor-sse")
	fmt.Println("   - API密钥: " + apiKey)
	fmt.Println("5. 点击保存")
	fmt.Println("6. 确保服务器处于启用状态")
	fmt.Println("==============================\n")
}

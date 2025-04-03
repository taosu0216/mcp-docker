package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/joho/godotenv"

	// 本地包导入
	"mcp-docker/client/pkg/mcp"
	"mcp-docker/client/pkg/model"
)

// 保存最后一次用户输入的命令
var lastUserCommand string

// 标记是否需要重试上一条命令
var shouldRetryCommand bool

// 全局客户端管理器
var clientManager *mcp.ClientManager

func main() {
	var err error

	fmt.Println("==== 云原生容器管理客户端启动 ====")
	fmt.Println("支持 Docker 和 Kubernetes 资源管理")
	time.Sleep(1 * time.Second)

	// 加载.env文件中的环境变量
	err = godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	// 添加全局恢复机制
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("\n程序崩溃:", r)
			fmt.Println("\n===== 错误详情 =====")
			debug.PrintStack()
			fmt.Println("\n请检查服务器是否正常运行，按Enter键退出...")
			bufio.NewReader(os.Stdin).ReadLine()
			os.Exit(1)
		}
	}()

	// 直接从环境变量获取服务器URL和API密钥
	serverURL := os.Getenv("MCP_SERVER_URL")

	fmt.Printf("使用服务器URL: %s\n", serverURL)

	// 创建根上下文
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	// 初始化客户端管理器
	clientManager = mcp.NewClientManager(serverURL)

	// 尝试初始化MCP工具
	fmt.Println("正在连接容器管理服务器...")
	var mcpTools []tool.BaseTool

	// 尝试通过直接调用getMCPTool初始化工具
	fmt.Println("正在获取容器管理工具...")
	mcpTools, err = mcp.GetMCPTools(ctx, clientManager, true)
	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "Unauthorized") {
			log.Fatalf("API认证失败: %v\n请检查.env文件中的API_KEY是否正确", err)
		} else {
			log.Fatalf("获取MCP工具失败: %v", err)
		}
	}

	fmt.Println("初始化聊天模型...")
	cm := model.NewChatModel(
		ctx,
		os.Getenv("OPENAI_API_KEY"),
		os.Getenv("OPENAI_BASE_URL"),
		os.Getenv("OPENAI_MODEL"),
	)

	// 创建重连监控goroutine
	toolsUpdatedChan := make(chan []tool.BaseTool, 1)
	go monitorReconnection(ctx, clientManager, toolsUpdatedChan)

	runner, err := react.NewAgent(ctx, &react.AgentConfig{
		Model: cm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: mcpTools,
		},
	})
	if err != nil {
		log.Fatalf("初始化Agent失败: %v", err)
	}

	// 开始交互循环
	startInteractionLoop(ctx, runner, mcpTools, toolsUpdatedChan)
}

// monitorReconnection 监控重连信号并处理重连
func monitorReconnection(ctx context.Context, clientManager *mcp.ClientManager, toolsUpdatedChan chan<- []tool.BaseTool) {
	for {
		select {
		case <-clientManager.GetReconnectChannel():
			fmt.Println("检测到连接重置信号，尝试重新初始化MCP工具...")

			// 等待一段时间再重连
			time.Sleep(2 * time.Second)

			// 尝试重新获取工具
			newTools, err := mcp.GetMCPTools(ctx, clientManager, true)
			if err != nil {
				fmt.Printf("重新连接MCP服务器失败: %v\n", err)
				continue
			}

			fmt.Println("MCP工具重新连接成功")

			// 通过通道发送更新的工具列表
			select {
			case toolsUpdatedChan <- newTools:
				// 成功发送工具更新
			default:
				// 通道满，丢弃更新
			}

			// 设置重试标志
			if lastUserCommand != "" {
				fmt.Println("检测到连接问题已解决，将自动重试上一次的命令...")
				shouldRetryCommand = true
			}
		case <-ctx.Done():
			return
		}
	}
}

// startInteractionLoop 开始用户交互循环
func startInteractionLoop(ctx context.Context, initialRunner *react.Agent, initialTools []tool.BaseTool, toolsUpdatedChan <-chan []tool.BaseTool) {
	// 初始化工具列表
	mcpTools := initialTools

	dialog := make([]*schema.Message, 0)
	dialog = append(dialog, &schema.Message{
		Role: schema.System,
		Content: `
作为云原生容器管理助手，你必须始终回复中文并且严格遵守以下规则：

# 系统能力
你可以管理Docker和Kubernetes资源，包括：
- Docker: 容器、镜像、网络、卷等资源管理
- Kubernetes: Pod、Deployment、Service、命名空间等资源管理

# 命令规则
0. 记住永远不能一次执行多个命令，你应该执行一个命令等待结果后才执行下一个命令
   不要使用会持续产生输出的命令，比如docker run镜像不加-d，比如watch等命令

1. 生成命令前必须：
   - 检查危险操作（删除、清理等）
   - 确认命令格式符合Windows要求，不要使用|或者grep等unix才有的命令
   - 危险命令必须按此格式确认："【安全提示】即将执行：xxx，是否继续？(Y/N)"

2. Docker命令构造规则：
   - 命令以docker开头
   - 使用反斜杠路径（如docker run -v C:\app:/app）
   - 禁止使用管道符、重定向等复杂操作
   - 不要使用unix相关的命令

3. Kubernetes命令规则：
   - 不要手动构造kubectl命令，使用提供的MCP工具
   - 操作前先检查相关资源是否存在
   - 命名空间敏感操作需要先确认命名空间
   - 删除资源操作需要二次确认

4. 错误处理原则：
   - 当命令执行失败时，用普通用户能理解的方式解释错误
   - 不要尝试自动修复需要权限的操作
   - 提供可能的解决方案

5. 关于操作超时：
   - 执行stop、restart、remove等操作时可能需要较长时间
   - 如果执行命令后长时间没有响应，可能是服务器处理超时
   - 建议用户再次查看资源状态确认操作是否成功

示例对话：
用户：删除所有停止的容器
你：【安全提示】即将执行：docker system prune -a，这将删除所有未使用的容器、镜像和网络，是否继续？(Y/N)

用户：查看所有的Kubernetes命名空间
你：我将获取所有Kubernetes命名空间列表。

用户：查看default命名空间中的所有Pod
你：我将获取default命名空间中的所有Pod列表。
`,
	})

	// 是否是对话开始
	isFirstMessage := true
	// 使用初始化的runner
	runner := initialRunner
	// 最后一次工具更新时间
	lastToolUpdateTime := time.Now()
	// 工具更新间隔(10分钟)
	toolUpdateInterval := 10 * time.Minute

	for { // 多轮对话，除非用户输入了 "exit"，否则一直循环
		// 检查是否有工具更新
		select {
		case updatedTools := <-toolsUpdatedChan:
			// 更新工具列表
			mcpTools = updatedTools
			fmt.Printf("[系统] 工具列表已更新，共 %d 个工具\n", len(mcpTools))
		default:
			// 没有工具更新，继续
		}

		if !isFirstMessage {
			fmt.Println()
		}
		isFirstMessage = false
		fmt.Println("You: ") // 提示轮到用户输入了

		var message string

		// 检查是否需要重试上一条命令
		if shouldRetryCommand && lastUserCommand != "" {
			message = lastUserCommand
			fmt.Println(message + " (自动重试)")
			shouldRetryCommand = false
		} else {
			scanner := bufio.NewScanner(os.Stdin) // 获取用户在命令行的输入
			for scanner.Scan() {
				message += scanner.Text()
				break
			}

			if err := scanner.Err(); err != nil {
				panic(err)
			}

			// 保存用户输入，以便在连接重置后重试
			if message != "" && message != "exit" {
				lastUserCommand = message
			}
		}

		if message == "exit" {
			return
		}

		dialog = append(dialog, &schema.Message{
			Role:    schema.User,
			Content: message,
		})

		// 只在以下情况更新工具和重新初始化Agent:
		// 1. 检测到连接重置或工具需要更新
		// 2. 超过工具更新间隔时间
		// 3. 用户直接输入"更新工具"命令
		needUpdateTools := time.Since(lastToolUpdateTime) > toolUpdateInterval ||
			message == "更新工具" || message == "刷新工具" || message == "重新连接" ||
			clientManager.NeedsReconnect()

		if needUpdateTools {
			fmt.Println("[系统] 正在更新容器管理工具...")

			var err error
			mcpTools, err = mcp.GetMCPTools(ctx, clientManager, false)
			if err != nil {
				fmt.Printf("[系统] 获取MCP工具失败: %v\n", err)
				fmt.Println("AI: 很抱歉，我无法连接到容器管理服务器，请检查服务器是否正常运行。")
				continue
			}

			// 更新最后一次工具更新时间
			lastToolUpdateTime = time.Now()

			// 重新初始化Agent
			runner, err = react.NewAgent(ctx, &react.AgentConfig{
				Model: model.NewChatModel(
					ctx,
					os.Getenv("OPENAI_API_KEY"),
					os.Getenv("OPENAI_BASE_URL"),
					os.Getenv("OPENAI_MODEL"),
				),
				ToolsConfig: compose.ToolsNodeConfig{
					Tools: mcpTools,
				},
			})
			if err != nil {
				fmt.Printf("[系统] 初始化Agent失败: %v\n", err)
				fmt.Println("AI: 很抱歉，我无法初始化对话模型，请检查API密钥是否正确。")
				continue
			}

			if message == "更新工具" || message == "刷新工具" || message == "重新连接" {
				fmt.Printf("[系统] 成功获取 %d 个容器管理工具，已完成更新\n", len(mcpTools))
				// 如果用户只是要求更新工具，跳过当前轮次的实际对话
				dialog = dialog[:len(dialog)-1] // 从对话历史中移除"更新工具"命令
				continue
			}
		}

		fmt.Println("AI: ")

		// 添加超时控制
		generateCtx, generateCancel := context.WithTimeout(ctx, 45*time.Second)
		var out *schema.Message
		var generateErr error

		done := make(chan bool)
		go func() {
			out, generateErr = runner.Generate(generateCtx, dialog, agent.WithComposeOptions())
			done <- true
		}()

		// 等待生成完成或超时
		select {
		case <-done:
			generateCancel()
			if generateErr != nil {
				// 检查是否是会话ID无效或超时问题
				if strings.Contains(generateErr.Error(), "connection") ||
					strings.Contains(generateErr.Error(), "timeout") ||
					strings.Contains(generateErr.Error(), "EOF") ||
					strings.Contains(generateErr.Error(), "Invalid session ID") {
					// 处理会话ID无效或超时问题
					fmt.Printf("\n[系统] 检测到连接问题，尝试重新连接MCP服务器...\n")
					clientManager.MarkConnectionFailed(generateErr)
					fmt.Println("很抱歉，连接服务器时出现问题，正在尝试重新连接，请稍后再试。")
					continue
				}

				fmt.Printf("\n[系统] 运行Agent失败: %v\n", generateErr)
				fmt.Println("我在处理您的请求时遇到了问题，请稍后再试或尝试不同的命令。")
				continue
			}

			// 提取和显示AI回应
			output := out.Content
			fmt.Println(output)

			// 添加AI回复到对话历史
			dialog = append(dialog, &schema.Message{
				Role:    schema.Assistant,
				Content: output,
			})

		case <-time.After(50 * time.Second):
			generateCancel()
			fmt.Println("\n[系统] 命令执行超时")
			fmt.Println("处理您的请求时间过长，可能是服务器响应缓慢或命令过于复杂。请尝试更简单的命令或稍后再试。")

			// 标记连接可能有问题
			clientManager.MarkConnectionFailed(fmt.Errorf("命令执行超时"))
			continue
		}

		// 如果对话历史过长，保留最近的对话
		// 保留系统消息和最近的对话记录，但最多保留30条消息以保持足够上下文
		if len(dialog) > 31 {
			// 保留系统消息和最近的15轮对话（31条消息）
			dialog = append(dialog[:1], dialog[len(dialog)-30:]...)
		}
	}
}

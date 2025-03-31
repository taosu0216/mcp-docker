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

	"github.com/cloudwego/eino-ext/components/model/openai"
	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	fmt.Println("==== Docker MCP 客户端启动 ====")
	time.Sleep(1 * time.Second)

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

	// 创建根上下文
	ctx := context.Background()

	// 尝试初始化MCP工具
	fmt.Println("正在连接Docker服务器...")
	var mcpTools []tool.BaseTool

	// 尝试通过直接调用getMCPTool初始化工具
	fmt.Println("正在获取Docker管理工具...")
	mcpTools = getMCPTool(ctx)

	fmt.Println("初始化聊天模型...")
	cm := getChatModel(ctx)

	runner, err := react.NewAgent(ctx, &react.AgentConfig{
		Model: cm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: mcpTools,
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	dialog := make([]*schema.Message, 0)
	dialog = append(dialog, &schema.Message{
		Role: schema.System,
		Content: `
作为Docker安全助手，你必须始终回复中文并且严格遵守以下规则：
0. 记住永远不能一次执行多个命令,你应该执行一个命令等待结果后才执行下一个命令;
   不要使用会持续产生输出的命令, 比如docker run 镜像不加 -d ,比如docker state	

1. 生成命令前必须：
   - 检查是否包含rm/delete/prune等关键词
   - 确认命令格式符合Windows要求,不要使用 | 或者 grep 等unix才有的命令
   - 危险命令必须按此格式确认："【安全提示】即将执行：docker xxx，是否继续？(Y/N)"

2. 命令构造规则：
   - 所有命令必须以docker开头
   - 使用反斜杠路径（如docker run -v C:\app:/app）
   - 禁止使用管道符、重定向等复杂操作
   - 不要使用unix相关的命令
   - 如果有多个命令需要执行,那就批次调用tool,不要一次使用多个命令
   - 不要使用 | 或者 grep 等unix才有的命令

3. 错误处理原则：
   - 当命令执行失败时，用普通用户能理解的方式解释错误
   - 不要尝试自动修复需要权限的操作

4. 关于容器操作超时：
   - 执行stop、restart、remove等操作时可能需要较长时间
   - 如果执行命令后长时间没有响应，可能是服务器处理超时
   - 建议用户查看容器状态确认操作是否成功

示例对话：
用户：删除所有停止的容器
你：【安全提示】即将执行：docker system prune -a，是否继续？(Y/N)

用户：查看镜像列表
你：执行命令：docker ps -a"
`,
	})
	flag := true
	for { // 多轮对话，除非用户输入了 "exit"，否则一直循环
		if !flag {
			fmt.Println()
		}
		flag = false
		fmt.Println("You: ") // 提示轮到用户输入了

		var message string
		scanner := bufio.NewScanner(os.Stdin) // 获取用户在命令行的输入
		for scanner.Scan() {
			message += scanner.Text()
			break
		}

		if err = scanner.Err(); err != nil {
			panic(err)
		}

		if message == "exit" {
			return
		}

		dialog = append(dialog, &schema.Message{
			Role:    schema.User,
			Content: message,
		})

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
				fmt.Println("命令执行失败:", generateErr)
				// 直接添加错误信息到对话中
				errorMsg := fmt.Sprintf("很抱歉，执行命令时遇到错误: %v\n可能是命令超时或服务器未响应，请重试或查看容器状态。", generateErr)
				dialog = append(dialog, &schema.Message{
					Role:    schema.Assistant,
					Content: errorMsg,
				})
				tokenf("%v", errorMsg)
				continue
			}
		case <-time.After(50 * time.Second):
			generateCancel()
			fmt.Println("命令执行超时")
			errorMsg := "很抱歉，命令执行超时。这可能是因为服务器处理时间过长或网络问题。\n建议通过 `docker ps` 查看容器状态来确认操作是否已完成。"
			dialog = append(dialog, &schema.Message{
				Role:    schema.Assistant,
				Content: errorMsg,
			})
			tokenf("%v", errorMsg)
			continue
		}

		fmt.Println("Answer:")
		outPut := out.Content
		outPut = strings.TrimSpace(outPut)

		tokenf("%v", outPut)
		dialog = append(dialog, &schema.Message{
			Role:    schema.Assistant,
			Content: outPut,
		})
	}
}

func getChatModel(ctx context.Context) model.ChatModel {
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  "********************",
		Model:   "deepseek-v3-241226",
		BaseURL: "https://ark.cn-beijing.volces.com/api/v3/",
	})
	if err != nil {
		log.Fatal(err)
	}
	return cm
}

func getMCPTool(ctx context.Context) []tool.BaseTool {
	// 使用根上下文而不是传入的上下文以避免连接过早关闭
	rootCtx := context.Background()

	fmt.Println("正在连接Docker MCP服务器...")

	cli, err := client.NewSSEMCPClient("http://localhost:12345/sse")
	if err != nil {
		fmt.Printf("创建MCP客户端失败: %v\n", err)
		log.Fatal(err)
	}

	// 增强的重试逻辑
	var startErr error
	for retries := 0; retries < 5; retries++ {
		startErrorChannel := make(chan error, 1)

		// 使用goroutine进行连接，避免卡住
		go func() {
			fmt.Printf("尝试启动MCP客户端 (%d/5)...\n", retries+1)
			startErrorChannel <- cli.Start(rootCtx)
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

		time.Sleep(2 * time.Second)
	}

	if startErr != nil {
		log.Fatalf("无法连接到MCP服务器，请确保服务端已启动: %v", startErr)
	}

	fmt.Println("初始化MCP客户端...")
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "docker-cli",
		Version: "1.0.0",
	}

	// 初始化请求使用一个单独的超时上下文
	initCtx, initCancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer initCancel()

	_, err = cli.Initialize(initCtx, initRequest)
	if err != nil {
		log.Fatalf("初始化MCP客户端失败: %v", err)
	}

	fmt.Println("获取Docker工具列表...")
	// 获取工具也使用单独的超时上下文
	toolsCtx, toolsCancel := context.WithTimeout(rootCtx, 10*time.Second)
	defer toolsCancel()

	tools, err := mcpp.GetTools(toolsCtx, &mcpp.Config{
		Cli: cli,
	})

	if err != nil {
		log.Fatalf("获取MCP工具失败: %v", err)
	}

	fmt.Printf("成功获取 %d 个Docker管理工具\n", len(tools))
	for i, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			log.Fatalf("获取MCP工具信息失败: %v", err)
		}
		fmt.Printf("  %d. %s\n", i+1, info.Name)
	}

	return tools
}

func tokenf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	fmt.Printf("%s%s%s", colorBrown, message, colorReset)
}

const (
	colorBrown = "\033[31;1m"
	colorReset = "\033[0m"
)

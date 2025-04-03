package mcp

import (
	"context"
	"fmt"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
)

// GetMCPTools 获取MCP工具列表
// verbose参数控制是否打印详细工具列表
func GetMCPTools(ctx context.Context, clientManager *ClientManager, verbose ...bool) ([]tool.BaseTool, error) {
	// 默认不显示详细信息
	showVerbose := false
	if len(verbose) > 0 && verbose[0] {
		showVerbose = true
	}

	// 获取MCP客户端
	cli, err := clientManager.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	if showVerbose {
		fmt.Println("获取Docker工具列表...")
	}

	// 获取工具使用单独的超时上下文
	toolsCtx, toolsCancel := context.WithTimeout(ctx, 10*time.Second)
	defer toolsCancel()

	tools, err := mcpp.GetTools(toolsCtx, &mcpp.Config{
		Cli: cli,
	})

	if err != nil {
		// 如果获取工具失败，标记连接为失败状态
		clientManager.MarkConnectionFailed(err)
		return nil, fmt.Errorf("获取MCP工具失败: %v", err)
	}

	if showVerbose {
		fmt.Printf("成功获取 %d 个 mcp 工具\n", len(tools))
		for i, t := range tools {
			info, err := t.Info(ctx)
			if err != nil {
				return nil, fmt.Errorf("获取MCP工具信息失败: %v", err)
			}
			fmt.Printf("  %d. %s\n", i+1, info.Name)
		}
	}

	return tools, nil
}

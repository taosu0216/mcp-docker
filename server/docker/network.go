package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/network"
	"github.com/mark3labs/mcp-go/mcp"
)

// 列出网络的工具函数
func ListNetworksTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: list_networks")

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取网络列表
	networks, err := cli.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取网络列表失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString("NETWORK ID\tNAME\tDRIVER\tSCOPE\n")
	for _, network := range networks {
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n",
			network.ID[:12],
			network.Name,
			network.Driver,
			network.Scope))
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 删除网络的工具函数
func RemoveNetworkTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	networkID := request.Params.Arguments["network_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: remove_network, network_id=", networkID)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 删除网络
	err = cli.NetworkRemove(ctx, networkID)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("删除网络失败: %v", err)), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("网络 %s 已成功删除", networkID)), nil
}

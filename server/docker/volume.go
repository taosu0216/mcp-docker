package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/volume"
	"github.com/mark3labs/mcp-go/mcp"
)

// 列出卷的工具函数
func ListVolumesTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: list_volumes")

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取卷列表
	volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取卷列表失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString("DRIVER\tVOLUME NAME\tMOUNTPOINT\tLABELS\n")
	for _, vol := range volumes.Volumes {
		if vol == nil {
			continue
		}

		// 格式化标签
		var labels []string
		for k, v := range vol.Labels {
			labels = append(labels, fmt.Sprintf("%s=%s", k, v))
		}
		labelsStr := strings.Join(labels, ",")
		if labelsStr == "" {
			labelsStr = "<none>"
		}

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n",
			vol.Driver,
			vol.Name,
			vol.Mountpoint,
			labelsStr))
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 删除卷的工具函数
func RemoveVolumeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	volumeName := request.Params.Arguments["volume_name"].(string)

	fmt.Println("ai 正在调用mcp server的tool: remove_volume, volume_name=", volumeName)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 删除卷
	err = cli.VolumeRemove(ctx, volumeName, false)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("删除卷失败: %v", err)), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("卷 %s 已成功删除", volumeName)), nil
}

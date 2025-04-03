package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/filters"
	"github.com/mark3labs/mcp-go/mcp"
)

// 系统清理的响应结构体
type SystemPruneReport struct {
	ContainersDeleted []string
	ImagesDeleted     []map[string]string
	SpaceReclaimed    uint64
}

// 系统信息工具函数
func SystemInfoTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: system_info")

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取系统信息
	info, err := cli.Info(ctx)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取系统信息失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Docker版本: %s\n", info.ServerVersion))
	result.WriteString(fmt.Sprintf("容器数量: %d (运行中: %d, 已暂停: %d, 已停止: %d)\n",
		info.Containers, info.ContainersRunning, info.ContainersPaused, info.ContainersStopped))
	result.WriteString(fmt.Sprintf("镜像数量: %d\n", info.Images))
	result.WriteString(fmt.Sprintf("驱动: %s\n", info.Driver))
	result.WriteString(fmt.Sprintf("操作系统: %s\n", info.OperatingSystem))
	result.WriteString(fmt.Sprintf("架构: %s\n", info.Architecture))
	result.WriteString(fmt.Sprintf("内核版本: %s\n", info.KernelVersion))
	result.WriteString(fmt.Sprintf("CPU: %d\n", info.NCPU))
	result.WriteString(fmt.Sprintf("内存: %s\n", FormatSize(uint64(info.MemTotal))))
	result.WriteString(fmt.Sprintf("Docker根目录: %s\n", info.DockerRootDir))
	result.WriteString(fmt.Sprintf("日志驱动: %s\n", info.LoggingDriver))
	result.WriteString(fmt.Sprintf("Cgroup驱动: %s\n", info.CgroupDriver))

	return mcp.NewToolResultText(result.String()), nil
}

// 系统清理工具函数
func SystemPruneTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all, _ := request.Params.Arguments["all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: system_prune, all=", all)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 手动实现系统清理功能
	pruneReport := SystemPruneReport{}

	// 清理未使用的容器
	containersPrune, err := cli.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("清理容器失败: %v", err)), err
	}
	pruneReport.ContainersDeleted = containersPrune.ContainersDeleted
	pruneReport.SpaceReclaimed += containersPrune.SpaceReclaimed

	// 清理未使用的网络
	_, err = cli.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("清理网络失败: %v", err)), err
	}

	// 清理未使用的镜像
	if all {
		imagesPrune, err := cli.ImagesPrune(ctx, filters.NewArgs())
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("清理镜像失败: %v", err)), err
		}

		// 转换镜像删除响应项
		for _, item := range imagesPrune.ImagesDeleted {
			imgItem := map[string]string{
				"Untagged": item.Untagged,
				"Deleted":  item.Deleted,
			}
			pruneReport.ImagesDeleted = append(pruneReport.ImagesDeleted, imgItem)
		}

		pruneReport.SpaceReclaimed += imagesPrune.SpaceReclaimed
	}

	// 清理未使用的卷
	volumesPrune, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("清理卷失败: %v", err)), err
	}
	pruneReport.SpaceReclaimed += volumesPrune.SpaceReclaimed

	// 格式化输出
	var result strings.Builder
	if len(pruneReport.ContainersDeleted) > 0 {
		result.WriteString("已删除的容器:\n")
		for _, container := range pruneReport.ContainersDeleted {
			result.WriteString(fmt.Sprintf("  %s\n", container))
		}
	} else {
		result.WriteString("没有容器被删除\n")
	}

	if len(pruneReport.ImagesDeleted) > 0 {
		result.WriteString("已删除的镜像:\n")
		for _, img := range pruneReport.ImagesDeleted {
			if img["Untagged"] != "" {
				result.WriteString(fmt.Sprintf("  取消标记: %s\n", img["Untagged"]))
			}
			if img["Deleted"] != "" {
				result.WriteString(fmt.Sprintf("  删除: %s\n", img["Deleted"]))
			}
		}
	} else {
		result.WriteString("没有镜像被删除\n")
	}

	result.WriteString(fmt.Sprintf("释放空间: %s\n", FormatSize(pruneReport.SpaceReclaimed)))

	return mcp.NewToolResultText(result.String()), nil
}

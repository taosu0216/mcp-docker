package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/mark3labs/mcp-go/mcp"
)

// 列出镜像的工具函数
func ListImagesTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	showAll, _ := request.Params.Arguments["show_all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: list_images, show_all=", showAll)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取镜像列表
	images, err := cli.ImageList(ctx, image.ListOptions{All: showAll})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取镜像列表失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString("REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE\n")
	for _, img := range images {
		var repo, tag string
		if len(img.RepoTags) > 0 && img.RepoTags[0] != "<none>:<none>" {
			for _, repoTag := range img.RepoTags {
				repo, tag = ParseRepoTag(repoTag)
				result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\n",
					repo,
					tag,
					img.ID[7:19],
					fmt.Sprintf("%d seconds ago", img.Created),
					FormatSize(uint64(img.Size))))
			}
		} else {
			result.WriteString(fmt.Sprintf("<none>\t<none>\t%s\t%s\t%s\n",
				img.ID[7:19],
				fmt.Sprintf("%d seconds ago", img.Created),
				FormatSize(uint64(img.Size))))
		}
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 删除镜像的工具函数
func RemoveImageTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	imageID := request.Params.Arguments["image_id"].(string)
	force, _ := request.Params.Arguments["force"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: remove_image, image_id=", imageID, ", force=", force)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 删除镜像
	_, err = cli.ImageRemove(ctx, imageID, image.RemoveOptions{
		Force:         force,
		PruneChildren: true,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("删除镜像失败: %v", err)), err
	}

	return mcp.NewToolResultText(fmt.Sprintf("镜像 %s 已成功删除", imageID)), nil
}

// 拉取镜像的工具函数
func PullImageTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	imageName := request.Params.Arguments["image_name"].(string)

	fmt.Println("ai 正在调用mcp server的tool: pull_image, image_name=", imageName)
	fmt.Println("开始拉取镜像，将显示实时进度...")

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 拉取镜像
	reader, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("拉取镜像失败: %v", err)), err
	}
	defer reader.Close()

	// 创建进度读取器
	progressReader := NewProgressReader(reader)
	progressReader.StartProgress()

	// 收集所有进度更新
	var progressOutput strings.Builder
	progressOutput.WriteString(fmt.Sprintf("开始拉取镜像: %s\n", imageName))
	fmt.Printf("开始拉取镜像: %s\n", imageName)

	// 显示进度更新
	for update := range progressReader.Updates {
		progressOutput.WriteString(update)
		// 实时打印进度到服务器控制台
		fmt.Print(update)
	}

	fmt.Println("镜像拉取完成!")

	return mcp.NewToolResultText(fmt.Sprintf("成功拉取镜像: %s\n\n%s", imageName, progressOutput.String())), nil
}

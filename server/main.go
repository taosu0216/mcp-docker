package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/joho/godotenv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"mcp-docker/server/docker"
	"mcp-docker/server/k8s"
)

// 系统清理的响应结构体
type SystemPruneReport struct {
	ContainersDeleted []string
	ImagesDeleted     []string
	SpaceReclaimed    uint64
}

func main() {
	var err error
	// 加载.env文件中的环境变量
	err = godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("======================================")
	fmt.Println("Docker & K8s MCP 服务器启动中...")
	fmt.Println("版本: 1.0.0")
	fmt.Println("======================================")

	// 获取配置参数
	address := os.Getenv("MCP_SERVER_ADDRESS")

	// 创建并配置MCP服务器
	svr := server.NewMCPServer("docker-k8s mcp server", mcp.LATEST_PROTOCOL_VERSION)

	fmt.Println()
	fmt.Println("======================================")
	fmt.Println("MCP服务器配置：")
	fmt.Println("无需鉴权，所有客户端都可以直接访问")
	fmt.Println("======================================")

	// 添加Docker容器相关工具
	svr.AddTool(mcp.NewTool("list_containers",
		mcp.WithDescription("列出所有容器"),
		mcp.WithBoolean("show_all",
			mcp.Description("是否显示所有容器，包括已停止的容器"),
		),
		mcp.WithString("api_key",
			mcp.Required(),
			mcp.Description("API密钥"),
		),
	), docker.ListContainersTool)

	svr.AddTool(mcp.NewTool("start_container",
		mcp.WithDescription("启动已停止的容器"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要启动的容器ID"),
		),
	), docker.StartContainerTool)

	svr.AddTool(mcp.NewTool("create_container",
		mcp.WithDescription("创建并运行一个新容器"),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("容器使用的镜像"),
		),
		mcp.WithString("name",
			mcp.Description("容器名称"),
		),
		mcp.WithArray("ports",
			mcp.Description("端口映射，格式为 [\"宿主机端口:容器端口\", ...]"),
		),
		mcp.WithArray("volumes",
			mcp.Description("卷挂载，格式为 [\"宿主机路径:容器路径\", ...]"),
		),
		mcp.WithArray("env",
			mcp.Description("环境变量，格式为 [\"KEY=VALUE\", ...]"),
		),
		mcp.WithString("command",
			mcp.Description("容器启动命令"),
		),
		mcp.WithBoolean("detach",
			mcp.Description("是否在后台运行"),
			mcp.DefaultBool(true),
		),
	), docker.CreateContainerTool)

	svr.AddTool(mcp.NewTool("stop_container",
		mcp.WithDescription("停止指定的容器"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要停止的容器ID"),
		),
	), docker.StopContainerTool)

	svr.AddTool(mcp.NewTool("remove_container",
		mcp.WithDescription("删除指定的容器"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要删除的容器ID"),
		),
		mcp.WithBoolean("force",
			mcp.Description("是否强制删除，即使容器正在运行"),
			mcp.DefaultBool(false),
		),
	), docker.RemoveContainerTool)

	svr.AddTool(mcp.NewTool("restart_container",
		mcp.WithDescription("重启指定的容器"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要重启的容器ID"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("停止容器前的等待时间（秒）"),
			mcp.DefaultNumber(1.0),
		),
	), docker.RestartContainerTool)

	svr.AddTool(mcp.NewTool("container_logs",
		mcp.WithDescription("查看容器日志"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要查看日志的容器ID"),
		),
		mcp.WithNumber("tail",
			mcp.Description("仅返回指定数量的日志行"),
			mcp.DefaultNumber(100.0),
		),
		mcp.WithBoolean("timestamps",
			mcp.Description("是否显示时间戳"),
			mcp.DefaultBool(false),
		),
	), docker.ContainerLogsTool)

	svr.AddTool(mcp.NewTool("inspect_container",
		mcp.WithDescription("查看容器详细信息"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要查看的容器ID"),
		),
	), docker.InspectContainerTool)

	svr.AddTool(mcp.NewTool("container_status",
		mcp.WithDescription("快速检查容器的运行状态"),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("要检查的容器ID"),
		),
	), docker.ContainerStatusTool)

	// 添加Docker镜像相关工具
	svr.AddTool(mcp.NewTool("list_images",
		mcp.WithDescription("列出所有镜像"),
		mcp.WithBoolean("show_all",
			mcp.Description("是否显示所有镜像，包括中间层镜像"),
			mcp.DefaultBool(false),
		),
	), docker.ListImagesTool)

	svr.AddTool(mcp.NewTool("remove_image",
		mcp.WithDescription("删除指定的镜像"),
		mcp.WithString("image_id",
			mcp.Required(),
			mcp.Description("要删除的镜像ID或名称"),
		),
		mcp.WithBoolean("force",
			mcp.Description("是否强制删除"),
			mcp.DefaultBool(false),
		),
	), docker.RemoveImageTool)

	svr.AddTool(mcp.NewTool("pull_image",
		mcp.WithDescription("拉取指定的镜像"),
		mcp.WithString("image_name",
			mcp.Required(),
			mcp.Description("要拉取的镜像名称"),
		),
	), docker.PullImageTool)

	// 添加Docker系统相关工具
	svr.AddTool(mcp.NewTool("system_info",
		mcp.WithDescription("显示Docker系统信息"),
	), docker.SystemInfoTool)

	svr.AddTool(mcp.NewTool("system_prune",
		mcp.WithDescription("清理未使用的Docker对象"),
		mcp.WithBoolean("all",
			mcp.Description("是否清理所有未使用的对象，包括未使用的镜像"),
			mcp.DefaultBool(false),
		),
	), docker.SystemPruneTool)

	// 添加Docker卷相关工具
	svr.AddTool(mcp.NewTool("list_volumes",
		mcp.WithDescription("列出所有卷"),
	), docker.ListVolumesTool)

	svr.AddTool(mcp.NewTool("remove_volume",
		mcp.WithDescription("删除指定的卷"),
		mcp.WithString("volume_name",
			mcp.Required(),
			mcp.Description("要删除的卷名称"),
		),
	), docker.RemoveVolumeTool)

	// 添加Docker网络相关工具
	svr.AddTool(mcp.NewTool("list_networks",
		mcp.WithDescription("列出所有网络"),
	), docker.ListNetworksTool)

	svr.AddTool(mcp.NewTool("remove_network",
		mcp.WithDescription("删除指定的网络"),
		mcp.WithString("network_id",
			mcp.Required(),
			mcp.Description("要删除的网络ID或名称"),
		),
	), docker.RemoveNetworkTool)

	// 添加Kubernetes Pod相关工具
	svr.AddTool(mcp.NewTool("list_pods",
		mcp.WithDescription("列出指定命名空间中的所有Pod"),
		mcp.WithString("namespace",
			mcp.Description("要查询的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.ListPodsTool)

	svr.AddTool(mcp.NewTool("describe_pod",
		mcp.WithDescription("查看Pod的详细信息"),
		mcp.WithString("pod_name",
			mcp.Required(),
			mcp.Description("要查看的Pod名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Pod所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.DescribePodTool)

	svr.AddTool(mcp.NewTool("delete_pod",
		mcp.WithDescription("删除指定的Pod"),
		mcp.WithString("pod_name",
			mcp.Required(),
			mcp.Description("要删除的Pod名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Pod所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
		mcp.WithBoolean("force",
			mcp.Description("是否强制删除"),
			mcp.DefaultBool(false),
		),
	), k8s.DeletePodTool)

	svr.AddTool(mcp.NewTool("pod_logs",
		mcp.WithDescription("获取Pod的日志"),
		mcp.WithString("pod_name",
			mcp.Required(),
			mcp.Description("要查看日志的Pod名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Pod所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
		mcp.WithString("container",
			mcp.Description("要查看日志的容器名称, 如果Pod中只有一个容器则可以省略"),
		),
		mcp.WithNumber("tail",
			mcp.Description("要查看的日志行数"),
			mcp.DefaultNumber(100.0),
		),
	), k8s.PodLogsTool)

	// 添加Kubernetes Deployment相关工具
	svr.AddTool(mcp.NewTool("list_deployments",
		mcp.WithDescription("列出指定命名空间中的所有Deployment"),
		mcp.WithString("namespace",
			mcp.Description("要查询的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.ListDeploymentsTool)

	svr.AddTool(mcp.NewTool("describe_deployment",
		mcp.WithDescription("查看Deployment的详细信息"),
		mcp.WithString("deployment_name",
			mcp.Required(),
			mcp.Description("要查看的Deployment名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Deployment所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.DescribeDeploymentTool)

	svr.AddTool(mcp.NewTool("scale_deployment",
		mcp.WithDescription("调整Deployment的副本数"),
		mcp.WithString("deployment_name",
			mcp.Required(),
			mcp.Description("要调整的Deployment名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Deployment所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
		mcp.WithNumber("replicas",
			mcp.Required(),
			mcp.Description("要设置的副本数"),
		),
	), k8s.ScaleDeploymentTool)

	svr.AddTool(mcp.NewTool("restart_deployment",
		mcp.WithDescription("重启Deployment的所有Pod"),
		mcp.WithString("deployment_name",
			mcp.Required(),
			mcp.Description("要重启的Deployment名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Deployment所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.RestartDeploymentTool)

	// 添加Kubernetes Service相关工具
	svr.AddTool(mcp.NewTool("list_services",
		mcp.WithDescription("列出指定命名空间中的所有Service"),
		mcp.WithString("namespace",
			mcp.Description("要查询的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.ListServicesTool)

	svr.AddTool(mcp.NewTool("describe_service",
		mcp.WithDescription("查看Service的详细信息"),
		mcp.WithString("service_name",
			mcp.Required(),
			mcp.Description("要查看的Service名称"),
		),
		mcp.WithString("namespace",
			mcp.Description("Service所在的命名空间, 默认为default"),
			mcp.DefaultString("default"),
		),
	), k8s.DescribeServiceTool)

	// 添加Kubernetes Namespace相关工具
	svr.AddTool(mcp.NewTool("list_namespaces",
		mcp.WithDescription("列出所有命名空间"),
	), k8s.ListNamespacesTool)

	svr.AddTool(mcp.NewTool("describe_namespace",
		mcp.WithDescription("查看命名空间的详细信息"),
		mcp.WithString("namespace_name",
			mcp.Required(),
			mcp.Description("要查看的命名空间名称"),
		),
	), k8s.DescribeNamespaceTool)

	svr.AddTool(mcp.NewTool("create_namespace",
		mcp.WithDescription("创建新的命名空间"),
		mcp.WithString("namespace_name",
			mcp.Required(),
			mcp.Description("要创建的命名空间名称"),
		),
	), k8s.CreateNamespaceTool)

	svr.AddTool(mcp.NewTool("delete_namespace",
		mcp.WithDescription("删除指定的命名空间"),
		mcp.WithString("namespace_name",
			mcp.Required(),
			mcp.Description("要删除的命名空间名称"),
		),
	), k8s.DeleteNamespaceTool)

	// 添加HTTP服务器
	httpServer := server.NewSSEServer(svr)

	// 启动服务器
	fmt.Printf("正在启动MCP服务器，监听地址: %s\n", address)
	err = http.ListenAndServe(address, httpServer)
	if err != nil {
		log.Fatal(err)
	}
}

// 创建Docker客户端的辅助函数
func createDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// 容器相关工具函数
func listContainersTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	showAll, _ := request.Params.Arguments["show_all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: list_containers, show_all=", showAll)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取容器列表
	options := container.ListOptions{All: showAll}
	containers, err := cli.ContainerList(ctx, options)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取容器列表失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString("CONTAINER ID\tIMAGE\tCOMMAND\tCREATED\tSTATUS\tPORTS\tNAMES\n")
	for _, container := range containers {
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			container.ID[:12],
			container.Image,
			container.Command,
			fmt.Sprintf("%d seconds ago", container.Created),
			container.Status,
			formatPorts(container.Ports),
			formatNames(container.Names)))
	}

	return mcp.NewToolResultText(result.String()), nil
}

func startContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: start_container, container_id=", containerID)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		err = cli.ContainerStart(timeoutCtx, containerID, container.StartOptions{})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err = <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("启动容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功启动", containerID)), nil
	case <-time.After(5 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("启动容器操作超时，但容器可能已启动。请使用 list_containers 检查状态")), nil
	}
}

// 实现带进度条的创建容器工具
func createContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	imageName := request.Params.Arguments["image"].(string)
	containerName, _ := request.Params.Arguments["name"].(string)
	portsArray, _ := request.Params.Arguments["ports"].([]interface{})
	volumesArray, _ := request.Params.Arguments["volumes"].([]interface{})
	envArray, _ := request.Params.Arguments["env"].([]interface{})
	cmd, _ := request.Params.Arguments["command"].(string)
	detach, _ := request.Params.Arguments["detach"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: create_container, image=", imageName)
	fmt.Println("开始创建容器，将显示实时进度...")

	// 创建Docker客户端
	cli, err := docker.CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 准备进度输出
	var progressOutput strings.Builder
	progressOutput.WriteString(fmt.Sprintf("开始创建容器，基于镜像：%s\n", imageName))
	fmt.Printf("开始创建容器，基于镜像：%s\n", imageName)

	// 实时显示进度的回调函数
	progressCallback := func(message string) {
		fmt.Print(message)
	}

	// 准备端口映射
	var message string
	message = "准备端口映射...\n"
	progressOutput.WriteString(message)
	progressCallback(message)

	portBindings := nat.PortMap{}
	exposedPorts := nat.PortSet{}

	for _, p := range portsArray {
		portMapping := p.(string)
		parts := strings.Split(portMapping, ":")
		if len(parts) == 2 {
			hostPort, containerPort := parts[0], parts[1]
			if !strings.Contains(containerPort, "/") {
				containerPort = containerPort + "/tcp"
			}
			natPort, _ := nat.NewPort("tcp", strings.TrimSuffix(containerPort, "/tcp"))

			portBindings[natPort] = append(portBindings[natPort], nat.PortBinding{
				HostIP:   "0.0.0.0",
				HostPort: hostPort,
			})
			exposedPorts[natPort] = struct{}{}

			message = fmt.Sprintf("  添加端口映射: %s:%s\n", hostPort, containerPort)
			progressOutput.WriteString(message)
			progressCallback(message)
		}
	}

	// 准备环境变量
	message = "准备环境变量...\n"
	progressOutput.WriteString(message)
	progressCallback(message)

	var env []string
	for _, e := range envArray {
		env = append(env, e.(string))
		message = fmt.Sprintf("  添加环境变量: %s\n", e.(string))
		progressOutput.WriteString(message)
		progressCallback(message)
	}

	// 准备卷映射
	message = "准备卷映射...\n"
	progressOutput.WriteString(message)
	progressCallback(message)

	var volumes []string
	for _, v := range volumesArray {
		volumes = append(volumes, v.(string))
		message = fmt.Sprintf("  添加卷映射: %s\n", v.(string))
		progressOutput.WriteString(message)
		progressCallback(message)
	}

	// 准备命令
	var cmdSlice []string
	if cmd != "" {
		cmdSlice = strings.Split(cmd, " ")
		message = fmt.Sprintf("设置启动命令: %s\n", cmd)
		progressOutput.WriteString(message)
		progressCallback(message)
	}

	// 创建容器配置
	config := &container.Config{
		Image:        imageName,
		Env:          env,
		Cmd:          cmdSlice,
		ExposedPorts: exposedPorts,
	}

	// 创建主机配置
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        volumes,
	}

	// 自定义的回调函数，将所有创建容器的进度信息转发到控制台
	containerCallback := func(update string) {
		progressCallback(update)
	}

	// 创建容器并获取进度
	containerID, createProgress, err := createContainerWithServerProgress(ctx, cli, config, hostConfig, containerName, detach, containerCallback)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("%s\n%v", progressOutput.String(), err)), err
	}

	// 合并进度输出
	progressOutput.WriteString(createProgress)

	fmt.Println("容器创建完成!")

	// 返回结果
	if detach {
		return mcp.NewToolResultText(fmt.Sprintf("容器已创建并启动，ID: %s\n\n%s", containerID, progressOutput.String())), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("容器已创建，ID: %s\n\n%s", containerID, progressOutput.String())), nil
}

// createContainerWithServerProgress 创建容器并实时显示进度
func createContainerWithServerProgress(ctx context.Context, cli *client.Client, config *container.Config, hostConfig *container.HostConfig, containerName string, detach bool, progressCallback func(string)) (string, string, error) {
	var progressOutput strings.Builder

	// 步骤跟踪
	step := 1
	totalSteps := 5 // 总共5个步骤：配置、创建、验证、启动(可选)、完成

	// 步骤1: 配置
	message := fmt.Sprintf("[%d/%d] 准备容器配置...\n", step, totalSteps)
	progressOutput.WriteString(message)
	if progressCallback != nil {
		progressCallback(message)
	}
	step++

	// 步骤2: 创建容器
	message = fmt.Sprintf("[%d/%d] 创建容器...\n", step, totalSteps)
	progressOutput.WriteString(message)
	if progressCallback != nil {
		progressCallback(message)
	}

	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", progressOutput.String(), fmt.Errorf("创建容器失败: %v", err)
	}
	step++

	// 步骤3: 验证
	message = fmt.Sprintf("[%d/%d] 验证容器...\n", step, totalSteps)
	progressOutput.WriteString(message)
	if progressCallback != nil {
		progressCallback(message)
	}
	step++

	// 如果需要启动容器
	if detach {
		// 步骤4: 启动容器
		message = fmt.Sprintf("[%d/%d] 启动容器...\n", step, totalSteps)
		progressOutput.WriteString(message)
		if progressCallback != nil {
			progressCallback(message)
		}

		err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
		if err != nil {
			return resp.ID, progressOutput.String(), fmt.Errorf("启动容器失败: %v", err)
		}

		// 等待一下，给容器启动一些时间
		time.Sleep(1 * time.Second)

		// 检查容器状态
		containerInfo, err := cli.ContainerInspect(ctx, resp.ID)
		if err == nil && containerInfo.State.Running {
			message = "容器成功启动并正在运行!\n"
			progressOutput.WriteString(message)
			if progressCallback != nil {
				progressCallback(message)
			}
		}

		step++
	}

	// 步骤5: 完成
	message = fmt.Sprintf("[%d/%d] 操作完成!\n", totalSteps, totalSteps)
	progressOutput.WriteString(message)
	if progressCallback != nil {
		progressCallback(message)
	}

	return resp.ID, progressOutput.String(), nil
}

func stopContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: stop_container, container_id=", containerID)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		timeout := 1 // 默认超时时间
		err := cli.ContainerStop(timeoutCtx, containerID, container.StopOptions{Timeout: &timeout})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err := <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("停止容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功停止", containerID)), nil
	case <-time.After(20 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("停止容器操作超时，但容器可能已停止。请使用 list_containers 检查状态")), nil
	}
}

func removeContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)
	force, _ := request.Params.Arguments["force"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: remove_container, container_id=", containerID, ", force=", force)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		options := container.RemoveOptions{Force: force}
		err := cli.ContainerRemove(timeoutCtx, containerID, options)
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err := <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("删除容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功删除", containerID)), nil
	case <-time.After(20 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("删除容器操作超时，但容器可能已被删除。请使用 list_containers 检查状态")), nil
	}
}

func restartContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	// 打印更详细的参数信息用于调试
	fmt.Println("restart_container参数详情:")
	for k, v := range request.Params.Arguments {
		fmt.Printf("  %s: 值=%v, 类型=%T\n", k, v, v)
	}

	// 尝试以不同的方式获取timeout参数
	var timeoutValue int = 10 // 默认值

	if timeout, ok := request.Params.Arguments["timeout"]; ok {
		fmt.Printf("找到timeout参数, 值=%v, 类型=%T\n", timeout, timeout)

		switch t := timeout.(type) {
		case float64:
			timeoutValue = int(t)
			fmt.Printf("转换timeout为int: %d (从float64)\n", timeoutValue)
		case int64:
			timeoutValue = int(t)
			fmt.Printf("转换timeout为int: %d (从int64)\n", timeoutValue)
		case int:
			timeoutValue = t
			fmt.Printf("使用timeout的int值: %d\n", timeoutValue)
		default:
			fmt.Printf("无法处理timeout类型 %T, 使用默认值: 10\n", t)
		}
	} else {
		fmt.Println("未找到timeout参数，使用默认值: 10")
	}

	fmt.Printf("ai 正在调用mcp server的tool: restart_container, container_id=%s, timeout=%d\n",
		containerID, timeoutValue)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		err := cli.ContainerRestart(timeoutCtx, containerID, container.StopOptions{Timeout: &timeoutValue})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err := <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("重启容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功重启", containerID)), nil
	case <-time.After(20 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("重启容器操作超时，但容器可能正在重启中。请使用 list_containers 检查状态")), nil
	}
}

func containerLogsTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)
	tail, _ := request.Params.Arguments["tail"].(int64)
	timestamps, _ := request.Params.Arguments["timestamps"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: container_logs, container_id=", containerID)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取容器日志
	tailStr := fmt.Sprintf("%d", tail)
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: timestamps,
		Tail:       tailStr,
	}

	reader, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取容器日志失败: %v", err)), err
	}
	defer reader.Close()

	// 读取容器日志
	buf := new(strings.Builder)
	_, err = io.Copy(buf, reader)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("读取容器日志失败: %v", err)), err
	}

	return mcp.NewToolResultText(buf.String()), nil
}

func containerStatusTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: container_status, container_id=", containerID)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan struct {
		info types.ContainerJSON
		err  error
	}, 1)

	// 在goroutine中运行容器检查
	go func() {
		info, err := cli.ContainerInspect(timeoutCtx, containerID)
		resultChan <- struct {
			info types.ContainerJSON
			err  error
		}{info, err}
	}()

	// 等待操作完成或超时
	select {
	case resultData := <-resultChan:
		if resultData.err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("检查容器状态失败: %v", resultData.err)), resultData.err
		}

		state := resultData.info.State
		var status string
		switch {
		case state.Running:
			status = "运行中"
		case state.Restarting:
			status = "重启中"
		case state.Paused:
			status = "已暂停"
		case state.Dead:
			status = "已死亡"
		case state.ExitCode != 0:
			status = fmt.Sprintf("已退出 (退出码: %d)", state.ExitCode)
		default:
			status = "已停止"
		}

		// 返回简洁的容器状态信息
		statusText := fmt.Sprintf("容器 %s (%s) 当前状态: %s\n",
			containerID[:12],
			strings.TrimPrefix(resultData.info.Name, "/"),
			status)

		// 添加健康检查信息（如果有）
		if state.Health != nil {
			statusText += fmt.Sprintf("健康状态: %s\n", state.Health.Status)
			if len(state.Health.Log) > 0 {
				lastLog := state.Health.Log[len(state.Health.Log)-1]
				statusText += fmt.Sprintf("最后检查: %s\n", lastLog.End.Format("2006-01-02 15:04:05"))
				statusText += fmt.Sprintf("退出码: %d\n", lastLog.ExitCode)
				if lastLog.ExitCode != 0 {
					statusText += fmt.Sprintf("错误: %s\n", lastLog.Output)
				}
			}
		}

		// 解析时间字符串
		if state.Running {
			startTime, err := time.Parse(time.RFC3339Nano, state.StartedAt)
			if err == nil {
				uptime := time.Since(startTime)
				statusText += fmt.Sprintf("已运行: %s\n", formatDuration(uptime))
				statusText += fmt.Sprintf("启动于: %s\n", startTime.Format("2006-01-02 15:04:05"))
			} else {
				statusText += fmt.Sprintf("启动于: %s\n", state.StartedAt)
			}
		} else if state.FinishedAt != "0001-01-01T00:00:00Z" {
			finishTime, err := time.Parse(time.RFC3339Nano, state.FinishedAt)
			if err == nil {
				statusText += fmt.Sprintf("结束于: %s\n", finishTime.Format("2006-01-02 15:04:05"))
			} else {
				statusText += fmt.Sprintf("结束于: %s\n", state.FinishedAt)
			}
		}

		return mcp.NewToolResultText(statusText), nil
	case <-time.After(10 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("获取容器状态超时，请稍后重试")), nil
	}
}

// 格式化时间间隔的辅助函数
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%d天%d小时%d分钟", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%d小时%d分钟", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分钟%d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}

func inspectContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: inspect_container, container_id=", containerID)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取容器详细信息
	containerInfo, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取容器详细信息失败: %v", err)), err
	}

	// 格式化输出重要信息
	var result strings.Builder

	result.WriteString(fmt.Sprintf("容器ID: %s\n", containerInfo.ID))
	result.WriteString(fmt.Sprintf("容器名称: %s\n", strings.TrimPrefix(containerInfo.Name, "/")))
	result.WriteString(fmt.Sprintf("镜像: %s\n", containerInfo.Image))
	result.WriteString(fmt.Sprintf("创建时间: %s\n", containerInfo.Created))
	result.WriteString(fmt.Sprintf("状态: %s\n", containerInfo.State.Status))

	if containerInfo.State.Running {
		result.WriteString(fmt.Sprintf("运行中: 是\n"))
		result.WriteString(fmt.Sprintf("开始时间: %s\n", containerInfo.State.StartedAt))
	} else {
		result.WriteString(fmt.Sprintf("运行中: 否\n"))
		if containerInfo.State.FinishedAt != "0001-01-01T00:00:00Z" {
			result.WriteString(fmt.Sprintf("结束时间: %s\n", containerInfo.State.FinishedAt))
		}
	}

	if containerInfo.State.ExitCode != 0 {
		result.WriteString(fmt.Sprintf("退出码: %d\n", containerInfo.State.ExitCode))
		if containerInfo.State.Error != "" {
			result.WriteString(fmt.Sprintf("错误: %s\n", containerInfo.State.Error))
		}
	}

	// 网络配置
	result.WriteString("\n网络配置:\n")
	for netName, netConfig := range containerInfo.NetworkSettings.Networks {
		result.WriteString(fmt.Sprintf("  网络名称: %s\n", netName))
		result.WriteString(fmt.Sprintf("  IP地址: %s\n", netConfig.IPAddress))
		result.WriteString(fmt.Sprintf("  网关: %s\n", netConfig.Gateway))
		result.WriteString(fmt.Sprintf("  Mac地址: %s\n", netConfig.MacAddress))
	}

	// 端口映射
	result.WriteString("\n端口映射:\n")
	for containerPort, hostPorts := range containerInfo.NetworkSettings.Ports {
		for _, hostPort := range hostPorts {
			result.WriteString(fmt.Sprintf("  %s -> %s:%s\n", containerPort, hostPort.HostIP, hostPort.HostPort))
		}
	}

	// 挂载点
	result.WriteString("\n挂载点:\n")
	for _, mount := range containerInfo.Mounts {
		result.WriteString(fmt.Sprintf("  类型: %s, 源: %s, 目标: %s\n", mount.Type, mount.Source, mount.Destination))
	}

	// 环境变量
	result.WriteString("\n环境变量:\n")
	for _, env := range containerInfo.Config.Env {
		result.WriteString(fmt.Sprintf("  %s\n", env))
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 镜像相关工具函数
func listImagesTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	showAll, _ := request.Params.Arguments["show_all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: list_images, show_all=", showAll)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取镜像列表
	options := image.ListOptions{All: showAll}
	images, err := cli.ImageList(ctx, options)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取镜像列表失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString("REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE\n")
	for _, img := range images {
		repotags := "<none>:<none>"
		if len(img.RepoTags) > 0 {
			repotags = img.RepoTags[0]
		}
		repo, tag := parseRepoTag(repotags)
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%d seconds ago\t%s\n",
			repo,
			tag,
			img.ID[7:19],
			img.Created,
			formatSize(uint64(img.Size))))
	}

	return mcp.NewToolResultText(result.String()), nil
}

func removeImageTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	imageID := request.Params.Arguments["image_id"].(string)
	force, _ := request.Params.Arguments["force"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: remove_image, image_id=", imageID, ", force=", force)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 删除镜像
	removed, err := cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: force})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("删除镜像失败: %v", err)), err
	}

	var result strings.Builder
	for _, r := range removed {
		if r.Untagged != "" {
			result.WriteString(fmt.Sprintf("Untagged: %s\n", r.Untagged))
		}
		if r.Deleted != "" {
			result.WriteString(fmt.Sprintf("Deleted: %s\n", r.Deleted))
		}
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 实现带进度条的拉取镜像工具
func pullImageTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	imageName := request.Params.Arguments["image_name"].(string)

	fmt.Println("ai 正在调用mcp server的tool: pull_image, image_name=", imageName)
	fmt.Println("开始拉取镜像，将显示实时进度...")

	// 创建Docker客户端
	cli, err := docker.CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 在服务器端打印进度的回调函数
	progressCallback := func(update string) {
		fmt.Print(update)
	}

	// 拉取镜像并获取进度输出
	progressOutput, err := pullImageWithServerProgress(ctx, cli, imageName, progressCallback)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("拉取镜像失败: %v", err)), err
	}

	fmt.Println("镜像拉取完成!")

	// 返回结果
	return mcp.NewToolResultText(fmt.Sprintf("成功拉取镜像: %s\n\n%s", imageName, progressOutput)), nil
}

// pullImageWithServerProgress 拉取镜像并在服务器端实时显示进度
func pullImageWithServerProgress(ctx context.Context, cli *client.Client, imageName string, progressCallback func(string)) (string, error) {
	// 拉取镜像
	reader, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// 创建进度读取器
	progressReader := docker.NewProgressReader(reader)
	progressReader.StartProgress()

	// 收集所有进度更新
	var progressOutput strings.Builder
	progressOutput.WriteString(fmt.Sprintf("开始拉取镜像: %s\n", imageName))

	// 如果传入了回调函数，立即调用它显示开始信息
	if progressCallback != nil {
		progressCallback(fmt.Sprintf("开始拉取镜像: %s\n", imageName))
	}

	// 显示进度更新
	for update := range progressReader.Updates {
		progressOutput.WriteString(update)

		// 如果传入了回调函数，实时显示进度
		if progressCallback != nil {
			progressCallback(update)
		}
	}

	return progressOutput.String(), nil
}

// 处理系统相关命令
func handleSystemCommands(ctx context.Context, cli *client.Client, args []string) (string, error) {
	if len(args) > 0 && args[0] == "prune" {
		// 处理 docker system prune 命令
		all := false
		for _, arg := range args {
			if arg == "-a" || arg == "--all" {
				all = true
				break
			}
		}

		// 执行系统清理 - 由于Docker Go SDK没有直接提供SystemPrune方法，我们需要手动实现
		// 清理容器
		var pruneReport SystemPruneReport
		containersPruneReport, err := cli.ContainersPrune(ctx, filters.NewArgs())
		if err != nil {
			return "", fmt.Errorf("容器清理失败: %v", err)
		}
		pruneReport.ContainersDeleted = containersPruneReport.ContainersDeleted
		pruneReport.SpaceReclaimed += containersPruneReport.SpaceReclaimed

		// 清理镜像（如果all=true）
		if all {
			imagesPruneReport, err := cli.ImagesPrune(ctx, filters.NewArgs())
			if err != nil {
				return "", fmt.Errorf("镜像清理失败: %v", err)
			}
			for _, img := range imagesPruneReport.ImagesDeleted {
				if img.Deleted != "" {
					pruneReport.ImagesDeleted = append(pruneReport.ImagesDeleted, img.Deleted)
				}
			}
			pruneReport.SpaceReclaimed += imagesPruneReport.SpaceReclaimed
		}

		// 清理卷
		volumesPruneReport, err := cli.VolumesPrune(ctx, filters.NewArgs())
		if err != nil {
			return "", fmt.Errorf("卷清理失败: %v", err)
		}
		pruneReport.SpaceReclaimed += volumesPruneReport.SpaceReclaimed

		// 清理网络
		_, err = cli.NetworksPrune(ctx, filters.NewArgs())
		if err != nil {
			return "", fmt.Errorf("网络清理失败: %v", err)
		}

		return fmt.Sprintf("已删除的容器: %d\n已删除的镜像: %d\n释放的空间: %s\n",
			len(pruneReport.ContainersDeleted),
			len(pruneReport.ImagesDeleted),
			formatSize(pruneReport.SpaceReclaimed)), nil
	} else if len(args) > 0 && args[0] == "info" {
		// 处理 docker system info 命令
		info, err := cli.Info(ctx)
		if err != nil {
			return "", fmt.Errorf("获取系统信息失败: %v", err)
		}

		return fmt.Sprintf("Docker信息:\n名称: %s\n容器数: %d\n运行中: %d\n已暂停: %d\n已停止: %d\n镜像数: %d\n",
			info.Name,
			info.Containers,
			info.ContainersRunning,
			info.ContainersPaused,
			info.ContainersStopped,
			info.Images), nil
	}

	return "", fmt.Errorf("不支持的系统命令: %v", args)
}

// 处理卷相关命令
func handleVolumeCommands(ctx context.Context, cli *client.Client, args []string) (string, error) {
	if len(args) > 0 && (args[0] == "ls" || args[0] == "list") {
		// 处理 docker volume ls 命令
		volumes, err := cli.VolumeList(ctx, volume.ListOptions{})
		if err != nil {
			return "", fmt.Errorf("获取卷列表失败: %v", err)
		}

		var result strings.Builder
		result.WriteString("DRIVER\tVOLUME NAME\n")
		for _, vol := range volumes.Volumes {
			result.WriteString(fmt.Sprintf("%s\t%s\n", vol.Driver, vol.Name))
		}
		return result.String(), nil
	} else if len(args) > 0 && args[0] == "rm" {
		// 处理 docker volume rm 命令
		if len(args) < 2 {
			return "", fmt.Errorf("缺少卷名称")
		}
		volumeName := args[1]

		err := cli.VolumeRemove(ctx, volumeName, false)
		if err != nil {
			return "", fmt.Errorf("删除卷失败: %v", err)
		}

		return fmt.Sprintf("卷 %s 已成功删除", volumeName), nil
	}

	return "", fmt.Errorf("不支持的卷命令: %v", args)
}

// 处理网络相关命令
func handleNetworkCommands(ctx context.Context, cli *client.Client, args []string) (string, error) {
	if len(args) > 0 && (args[0] == "ls" || args[0] == "list") {
		// 处理 docker network ls 命令
		networks, err := cli.NetworkList(ctx, network.ListOptions{})
		if err != nil {
			return "", fmt.Errorf("获取网络列表失败: %v", err)
		}

		var result strings.Builder
		result.WriteString("NETWORK ID\tNAME\tDRIVER\tSCOPE\n")
		for _, network := range networks {
			result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n",
				network.ID[:12],
				network.Name,
				network.Driver,
				network.Scope))
		}
		return result.String(), nil
	} else if len(args) > 0 && args[0] == "rm" {
		// 处理 docker network rm 命令
		if len(args) < 2 {
			return "", fmt.Errorf("缺少网络ID或名称")
		}
		networkID := args[1]

		err := cli.NetworkRemove(ctx, networkID)
		if err != nil {
			return "", fmt.Errorf("删除网络失败: %v", err)
		}

		return fmt.Sprintf("网络 %s 已成功删除", networkID), nil
	}

	return "", fmt.Errorf("不支持的网络命令: %v", args)
}

// 辅助函数
func formatPorts(ports []types.Port) string {
	var result []string
	for _, p := range ports {
		if p.PublicPort > 0 {
			result = append(result, fmt.Sprintf("%d:%d/%s", p.PublicPort, p.PrivatePort, p.Type))
		} else {
			result = append(result, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(result, ", ")
}

func formatNames(names []string) string {
	for i, name := range names {
		if len(name) > 0 && name[0] == '/' {
			names[i] = name[1:]
		}
	}
	return strings.Join(names, ", ")
}

func parseRepoTag(repoTag string) (string, string) {
	parts := strings.Split(repoTag, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return repoTag, "<none>"
}

func formatSize(size uint64) string {
	const (
		_          = iota
		KB float64 = 1 << (10 * iota)
		MB
		GB
		TB
	)

	var formatted string
	var unit string

	size64 := float64(size)

	switch {
	case size64 >= TB:
		formatted = fmt.Sprintf("%.2f", size64/TB)
		unit = "TB"
	case size64 >= GB:
		formatted = fmt.Sprintf("%.2f", size64/GB)
		unit = "GB"
	case size64 >= MB:
		formatted = fmt.Sprintf("%.2f", size64/MB)
		unit = "MB"
	case size64 >= KB:
		formatted = fmt.Sprintf("%.2f", size64/KB)
		unit = "KB"
	default:
		formatted = fmt.Sprintf("%.0f", size64)
		unit = "B"
	}

	return formatted + " " + unit
}

func systemInfoTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: system_info")

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取系统信息
	info, err := cli.Info(ctx)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取系统信息失败: %v", err)), err
	}

	result := fmt.Sprintf("Docker信息:\n名称: %s\n容器数: %d\n运行中: %d\n已暂停: %d\n已停止: %d\n镜像数: %d\n",
		info.Name,
		info.Containers,
		info.ContainersRunning,
		info.ContainersPaused,
		info.ContainersStopped,
		info.Images)

	return mcp.NewToolResultText(result), nil
}

func systemPruneTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all, _ := request.Params.Arguments["all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: system_prune, all=", all)

	// 创建Docker客户端
	cli, err := createDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 执行系统清理 - 由于Docker Go SDK没有直接提供SystemPrune方法，我们需要手动实现
	// 清理容器
	var pruneReport SystemPruneReport
	containersPruneReport, err := cli.ContainersPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("容器清理失败: %v", err)), err
	}
	pruneReport.ContainersDeleted = containersPruneReport.ContainersDeleted
	pruneReport.SpaceReclaimed += containersPruneReport.SpaceReclaimed

	// 清理镜像（如果all=true）
	if all {
		imagesPruneReport, err := cli.ImagesPrune(ctx, filters.NewArgs())
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("镜像清理失败: %v", err)), err
		}
		for _, img := range imagesPruneReport.ImagesDeleted {
			if img.Deleted != "" {
				pruneReport.ImagesDeleted = append(pruneReport.ImagesDeleted, img.Deleted)
			}
		}
		pruneReport.SpaceReclaimed += imagesPruneReport.SpaceReclaimed
	}

	// 清理卷
	volumesPruneReport, err := cli.VolumesPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("卷清理失败: %v", err)), err
	}
	pruneReport.SpaceReclaimed += volumesPruneReport.SpaceReclaimed

	// 清理网络
	_, err = cli.NetworksPrune(ctx, filters.NewArgs())
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("网络清理失败: %v", err)), err
	}

	result := fmt.Sprintf("已删除的容器: %d\n已删除的镜像: %d\n释放的空间: %s\n",
		len(pruneReport.ContainersDeleted),
		len(pruneReport.ImagesDeleted),
		formatSize(pruneReport.SpaceReclaimed))

	return mcp.NewToolResultText(result), nil
}

func listVolumesTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: list_volumes")

	// 创建Docker客户端
	cli, err := createDockerClient()
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
	result.WriteString("DRIVER\tVOLUME NAME\n")
	for _, vol := range volumes.Volumes {
		result.WriteString(fmt.Sprintf("%s\t%s\n", vol.Driver, vol.Name))
	}

	return mcp.NewToolResultText(result.String()), nil
}

func removeVolumeTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	volumeName := request.Params.Arguments["volume_name"].(string)

	fmt.Println("ai 正在调用mcp server的tool: remove_volume, volume_name=", volumeName)

	// 创建Docker客户端
	cli, err := createDockerClient()
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

func listNetworksTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fmt.Println("ai 正在调用mcp server的tool: list_networks")

	// 创建Docker客户端
	cli, err := createDockerClient()
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

func removeNetworkTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	networkID := request.Params.Arguments["network_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: remove_network, network_id=", networkID)

	// 创建Docker客户端
	cli, err := createDockerClient()
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

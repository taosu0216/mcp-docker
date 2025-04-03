package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/mark3labs/mcp-go/mcp"
)

// 列出容器的工具函数
func ListContainersTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	showAll, _ := request.Params.Arguments["show_all"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: list_containers, show_all=", showAll)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
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
			FormatPorts(container.Ports),
			FormatNames(container.Names)))
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 启动容器的工具函数
func StartContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: start_container, container_id=", containerID)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := CreateDockerClient()
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

// 创建容器的工具函数
func CreateContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 准备进度输出
	var progressOutput strings.Builder
	progressOutput.WriteString(fmt.Sprintf("开始创建容器，基于镜像：%s\n", imageName))
	fmt.Printf("开始创建容器，基于镜像：%s\n", imageName)

	// 准备端口映射
	message := "准备端口映射...\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

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

			detail := fmt.Sprintf("  添加端口映射: %s:%s\n", hostPort, containerPort)
			progressOutput.WriteString(detail)
			fmt.Print(detail)
		}
	}

	// 准备环境变量
	message = "准备环境变量...\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

	var env []string
	for _, e := range envArray {
		env = append(env, e.(string))
		detail := fmt.Sprintf("  添加环境变量: %s\n", e.(string))
		progressOutput.WriteString(detail)
		fmt.Print(detail)
	}

	// 准备卷映射
	message = "准备卷映射...\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

	var volumes []string
	for _, v := range volumesArray {
		volumes = append(volumes, v.(string))
		detail := fmt.Sprintf("  添加卷映射: %s\n", v.(string))
		progressOutput.WriteString(detail)
		fmt.Print(detail)
	}

	// 准备命令
	var cmdSlice []string
	if cmd != "" {
		cmdSlice = strings.Split(cmd, " ")
		detail := fmt.Sprintf("设置启动命令: %s\n", cmd)
		progressOutput.WriteString(detail)
		fmt.Print(detail)
	}

	// 创建容器配置
	message = "创建容器配置...\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

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

	// 创建网络配置
	networkConfig := &network.NetworkingConfig{}

	// 创建容器
	message = "创建容器中...\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

	resp, err := cli.ContainerCreate(
		ctx,
		config,
		hostConfig,
		networkConfig,
		nil,
		containerName,
	)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建容器失败: %v", err)), err
	}

	message = fmt.Sprintf("容器创建成功，ID: %s\n", resp.ID)
	progressOutput.WriteString(message)
	fmt.Print(message)

	// 如果设置了分离模式，启动容器
	if detach {
		message = "正在启动容器...\n"
		progressOutput.WriteString(message)
		fmt.Print(message)

		err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("容器创建成功，但启动失败: %v", err)), err
		}

		// 等待一下，给容器启动一些时间
		time.Sleep(1 * time.Second)

		// 检查容器状态
		containerInfo, err := cli.ContainerInspect(ctx, resp.ID)
		if err == nil && containerInfo.State.Running {
			message = "容器成功启动并正在运行!\n"
			progressOutput.WriteString(message)
			fmt.Print(message)
		}
	}

	message = "操作完成!\n"
	progressOutput.WriteString(message)
	fmt.Print(message)

	fmt.Println("容器创建完成!")

	// 返回结果
	if detach {
		return mcp.NewToolResultText(fmt.Sprintf("容器已创建并启动，ID: %s\n\n%s", resp.ID, progressOutput.String())), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("容器已创建，ID: %s\n\n%s", resp.ID, progressOutput.String())), nil
}

// 停止容器的工具函数
func StopContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: stop_container, container_id=", containerID)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		err = cli.ContainerStop(timeoutCtx, containerID, container.StopOptions{})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err = <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("停止容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功停止", containerID)), nil
	case <-time.After(15 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("停止容器操作超时，但容器可能已停止。请使用 list_containers 检查状态")), nil
	}
}

// 删除容器的工具函数
func RemoveContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)
	force, _ := request.Params.Arguments["force"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: remove_container, container_id=", containerID, ", force=", force)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		err = cli.ContainerRemove(timeoutCtx, containerID, container.RemoveOptions{
			Force: force,
		})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err = <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("删除容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功删除", containerID)), nil
	case <-time.After(15 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("删除容器操作超时，但容器可能已删除。请使用 list_containers 检查状态")), nil
	}
}

// 重启容器的工具函数
func RestartContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)
	timeout, _ := request.Params.Arguments["timeout"].(float64)

	fmt.Println("ai 正在调用mcp server的tool: restart_container, container_id=", containerID, ", timeout=", timeout)

	// 创建带超时的上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 创建一个结果通道
	resultChan := make(chan error, 1)

	// 在goroutine中运行容器操作
	go func() {
		err = cli.ContainerRestart(timeoutCtx, containerID, container.StopOptions{
			Timeout: IntPtr(int(timeout)),
		})
		resultChan <- err
	}()

	// 等待操作完成或超时
	select {
	case err = <-resultChan:
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("重启容器失败: %v", err)), err
		}
		return mcp.NewToolResultText(fmt.Sprintf("容器 %s 已成功重启", containerID)), nil
	case <-time.After(35 * time.Second):
		return mcp.NewToolResultText(fmt.Sprintf("重启容器操作超时，但容器可能已重启。请使用 list_containers 检查状态")), nil
	}
}

// 查看容器日志的工具函数
func ContainerLogsTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)
	tail, _ := request.Params.Arguments["tail"].(float64)
	timestamps, _ := request.Params.Arguments["timestamps"].(bool)

	fmt.Println("ai 正在调用mcp server的tool: container_logs, container_id=", containerID, ", tail=", tail)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	tailStr := fmt.Sprintf("%d", int(tail))
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: timestamps,
		Tail:       tailStr,
	}

	// 获取日志
	logs, err := cli.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("获取容器日志失败: %v", err)), err
	}
	defer logs.Close()

	// 读取日志内容
	logBytes, err := io.ReadAll(logs)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("读取容器日志失败: %v", err)), err
	}

	return mcp.NewToolResultText(string(logBytes)), nil
}

// 检查容器状态的工具函数
func ContainerStatusTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: container_status, container_id=", containerID)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取容器信息
	container, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("检查容器状态失败: %v", err)), err
	}

	// 格式化输出
	var result strings.Builder
	result.WriteString(fmt.Sprintf("容器 ID: %s\n", container.ID[:12]))
	result.WriteString(fmt.Sprintf("名称: %s\n", strings.TrimPrefix(container.Name, "/")))
	result.WriteString(fmt.Sprintf("状态: %s\n", container.State.Status))

	if container.State.Running {
		startTime, _ := time.Parse(time.RFC3339Nano, container.State.StartedAt)
		result.WriteString(fmt.Sprintf("已运行: %s\n", FormatDuration(time.Since(startTime))))
		result.WriteString(fmt.Sprintf("启动时间: %s\n", startTime.Format("2006-01-02 15:04:05")))
	} else if container.State.Dead {
		result.WriteString("容器已死亡\n")
	} else if container.State.Paused {
		result.WriteString("容器已暂停\n")
	} else if container.State.Restarting {
		result.WriteString("容器正在重启\n")
	} else {
		finishTime, _ := time.Parse(time.RFC3339Nano, container.State.FinishedAt)
		result.WriteString(fmt.Sprintf("退出时间: %s\n", finishTime.Format("2006-01-02 15:04:05")))
		if container.State.ExitCode != 0 {
			result.WriteString(fmt.Sprintf("退出代码: %d\n", container.State.ExitCode))
			result.WriteString(fmt.Sprintf("错误信息: %s\n", container.State.Error))
		}
	}

	result.WriteString(fmt.Sprintf("镜像: %s\n", container.Config.Image))
	result.WriteString(fmt.Sprintf("命令: %s\n", strings.Join(container.Config.Cmd, " ")))

	// 添加端口信息
	if len(container.NetworkSettings.Ports) > 0 {
		result.WriteString("端口映射:\n")
		for port, bindings := range container.NetworkSettings.Ports {
			for _, binding := range bindings {
				result.WriteString(fmt.Sprintf("  %s -> %s:%s\n", port, binding.HostIP, binding.HostPort))
			}
		}
	}

	// 添加卷挂载信息
	if len(container.Mounts) > 0 {
		result.WriteString("卷挂载:\n")
		for _, mount := range container.Mounts {
			result.WriteString(fmt.Sprintf("  %s -> %s (%s)\n", mount.Source, mount.Destination, mount.Type))
		}
	}

	return mcp.NewToolResultText(result.String()), nil
}

// 查看容器详细信息的工具函数
func InspectContainerTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	containerID := request.Params.Arguments["container_id"].(string)

	fmt.Println("ai 正在调用mcp server的tool: inspect_container, container_id=", containerID)

	// 创建Docker客户端
	cli, err := CreateDockerClient()
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("创建Docker客户端失败: %v", err)), err
	}
	defer cli.Close()

	// 获取容器信息
	container, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("检查容器详情失败: %v", err)), err
	}

	// 格式化完整的容器详情
	var result strings.Builder
	result.WriteString(fmt.Sprintf("容器ID: %s\n", container.ID))
	result.WriteString(fmt.Sprintf("创建时间: %s\n", container.Created))
	result.WriteString(fmt.Sprintf("状态:\n"))
	result.WriteString(fmt.Sprintf("  运行状态: %s\n", container.State.Status))
	result.WriteString(fmt.Sprintf("  运行中: %v\n", container.State.Running))
	result.WriteString(fmt.Sprintf("  暂停: %v\n", container.State.Paused))
	result.WriteString(fmt.Sprintf("  重启中: %v\n", container.State.Restarting))
	result.WriteString(fmt.Sprintf("  OOM: %v\n", container.State.OOMKilled))
	result.WriteString(fmt.Sprintf("  死亡: %v\n", container.State.Dead))
	result.WriteString(fmt.Sprintf("  PID: %d\n", container.State.Pid))
	result.WriteString(fmt.Sprintf("  退出代码: %d\n", container.State.ExitCode))
	result.WriteString(fmt.Sprintf("  错误: %s\n", container.State.Error))
	result.WriteString(fmt.Sprintf("  启动时间: %s\n", container.State.StartedAt))
	result.WriteString(fmt.Sprintf("  结束时间: %s\n", container.State.FinishedAt))
	result.WriteString(fmt.Sprintf("镜像: %s\n", container.Image))
	result.WriteString(fmt.Sprintf("重启策略: %s\n", container.HostConfig.RestartPolicy.Name))
	result.WriteString(fmt.Sprintf("网络模式: %s\n", container.HostConfig.NetworkMode))

	// 网络设置
	result.WriteString("网络设置:\n")
	for netName, netInfo := range container.NetworkSettings.Networks {
		result.WriteString(fmt.Sprintf("  网络: %s\n", netName))
		result.WriteString(fmt.Sprintf("    IP地址: %s\n", netInfo.IPAddress))
		result.WriteString(fmt.Sprintf("    网关: %s\n", netInfo.Gateway))
		result.WriteString(fmt.Sprintf("    MAC地址: %s\n", netInfo.MacAddress))
	}

	// 端口映射
	result.WriteString("端口映射:\n")
	for port, bindings := range container.NetworkSettings.Ports {
		if len(bindings) == 0 {
			result.WriteString(fmt.Sprintf("  %s: <未映射>\n", port))
			continue
		}
		for _, binding := range bindings {
			result.WriteString(fmt.Sprintf("  %s -> %s:%s\n", port, binding.HostIP, binding.HostPort))
		}
	}

	// 挂载点
	result.WriteString("挂载点:\n")
	for _, mount := range container.Mounts {
		result.WriteString(fmt.Sprintf("  类型: %s\n", mount.Type))
		result.WriteString(fmt.Sprintf("  源: %s\n", mount.Source))
		result.WriteString(fmt.Sprintf("  目标: %s\n", mount.Destination))
		result.WriteString(fmt.Sprintf("  读写模式: %s\n", mount.Mode))
		result.WriteString(fmt.Sprintf("  RW: %v\n", mount.RW))
		result.WriteString("\n")
	}

	// 配置
	result.WriteString("配置:\n")
	result.WriteString(fmt.Sprintf("  主机名: %s\n", container.Config.Hostname))
	result.WriteString(fmt.Sprintf("  域名: %s\n", container.Config.Domainname))
	result.WriteString(fmt.Sprintf("  用户: %s\n", container.Config.User))
	result.WriteString(fmt.Sprintf("  工作目录: %s\n", container.Config.WorkingDir))

	result.WriteString("  环境变量:\n")
	for _, env := range container.Config.Env {
		result.WriteString(fmt.Sprintf("    %s\n", env))
	}

	result.WriteString(fmt.Sprintf("  命令: %s\n", strings.Join(container.Config.Cmd, " ")))
	result.WriteString(fmt.Sprintf("  入口点: %s\n", strings.Join(container.Config.Entrypoint, " ")))

	return mcp.NewToolResultText(result.String()), nil
}

// 创建一个整数指针
func IntPtr(i int) *int {
	return &i
}

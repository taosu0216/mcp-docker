package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
)

// 创建Docker客户端的辅助函数
func CreateDockerClient() (*client.Client, error) {
	return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
}

// 格式化端口信息的辅助函数
func FormatPorts(ports []types.Port) string {
	if len(ports) == 0 {
		return ""
	}

	var result []string
	for _, port := range ports {
		if port.PublicPort > 0 {
			result = append(result, fmt.Sprintf("%s:%d->%d/%s", port.IP, port.PublicPort, port.PrivatePort, port.Type))
		} else {
			result = append(result, fmt.Sprintf("%d/%s", port.PrivatePort, port.Type))
		}
	}
	return strings.Join(result, ", ")
}

// 格式化容器名称的辅助函数
func FormatNames(names []string) string {
	if len(names) == 0 {
		return ""
	}

	var result []string
	for _, name := range names {
		result = append(result, strings.TrimPrefix(name, "/"))
	}
	return strings.Join(result, ", ")
}

// 解析仓库标签的辅助函数
func ParseRepoTag(repoTag string) (string, string) {
	parts := strings.Split(repoTag, ":")
	if len(parts) > 1 {
		return parts[0], parts[1]
	}
	return parts[0], "latest"
}

// 格式化大小的辅助函数
func FormatSize(size uint64) string {
	const (
		B  = 1
		KB = 1024 * B
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	var suffix string
	var value float64

	switch {
	case size >= TB:
		suffix = "TB"
		value = float64(size) / TB
	case size >= GB:
		suffix = "GB"
		value = float64(size) / GB
	case size >= MB:
		suffix = "MB"
		value = float64(size) / MB
	case size >= KB:
		suffix = "KB"
		value = float64(size) / KB
	default:
		suffix = "B"
		value = float64(size)
	}

	return fmt.Sprintf("%.2f %s", value, suffix)
}

// 格式化持续时间的辅助函数
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second

	if days > 0 {
		return fmt.Sprintf("%dd%dh%dm%ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// 进度显示相关功能 ----------------------------------------

// ImagePullProgress 用于解析Docker进度JSON
type ImagePullProgress struct {
	Status         string `json:"status"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
	Progress string `json:"progress"`
	ID       string `json:"id"`
}

// ProgressReader 是一个结构，用于追踪和处理Docker操作的进度
type ProgressReader struct {
	Reader        io.ReadCloser
	BytesRead     int64
	TotalBytes    int64
	LayerProgress map[string]*ImagePullProgress
	mu            sync.Mutex
	Updates       chan string
}

// NewProgressReader 创建一个新的进度读取器
func NewProgressReader(reader io.ReadCloser) *ProgressReader {
	return &ProgressReader{
		Reader:        reader,
		LayerProgress: make(map[string]*ImagePullProgress),
		Updates:       make(chan string, 10),
	}
}

// StartProgress 开始追踪进度
func (pr *ProgressReader) StartProgress() {
	go func() {
		defer close(pr.Updates)

		decoder := json.NewDecoder(pr.Reader)
		lastUpdateTime := time.Now()
		updateInterval := time.Millisecond * 500 // 每500毫秒更新一次

		for {
			var progress ImagePullProgress
			if err := decoder.Decode(&progress); err != nil {
				if err == io.EOF {
					// 正常结束
					pr.Updates <- "\n操作完成！"
					break
				}
				pr.Updates <- fmt.Sprintf("\n读取进度时出错: %v", err)
				break
			}

			// 更新进度信息
			pr.mu.Lock()
			if progress.ID != "" {
				pr.LayerProgress[progress.ID] = &progress
			}
			pr.mu.Unlock()

			// 定期更新进度，避免过于频繁的更新
			if time.Since(lastUpdateTime) > updateInterval {
				pr.updateProgress()
				lastUpdateTime = time.Now()
			}
		}
	}()
}

// updateProgress 更新并发送进度信息
func (pr *ProgressReader) updateProgress() {
	pr.mu.Lock()
	defer pr.mu.Unlock()

	var (
		message       strings.Builder
		totalCurrent  int64
		totalExpected int64
	)

	// 统计总进度
	for id, layer := range pr.LayerProgress {
		if layer.ProgressDetail.Total > 0 {
			totalCurrent += layer.ProgressDetail.Current
			totalExpected += layer.ProgressDetail.Total
		}

		// 添加每个层的进度信息
		if layer.Status != "" {
			// 截取ID以避免过长
			shortID := id
			if len(id) > 12 {
				shortID = id[:12]
			}
			fmt.Fprintf(&message, "[%s] %s %s\n", shortID, layer.Status, layer.Progress)
		}
	}

	// 计算总体进度百分比
	if totalExpected > 0 {
		percentage := float64(totalCurrent) / float64(totalExpected) * 100
		fmt.Fprintf(&message, "总体进度: %.2f%%\n", percentage)
	}

	// 发送进度更新
	if message.Len() > 0 {
		pr.Updates <- message.String()
	}
}

// PullImageWithProgress 拉取镜像并显示进度
func PullImageWithProgress(ctx context.Context, cli *client.Client, imageName string) (string, error) {
	// 拉取镜像
	reader, err := cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return "", err
	}
	defer reader.Close()

	// 创建进度读取器
	progressReader := NewProgressReader(reader)
	progressReader.StartProgress()

	// 收集所有进度更新
	var progressOutput strings.Builder
	progressOutput.WriteString(fmt.Sprintf("开始拉取镜像: %s\n", imageName))

	// 显示进度更新
	for update := range progressReader.Updates {
		progressOutput.WriteString(update)
	}

	return progressOutput.String(), nil
}

// CreateContainerWithProgress 创建容器并显示进度步骤
func CreateContainerWithProgress(ctx context.Context, cli *client.Client, config *container.Config, hostConfig *container.HostConfig, containerName string, detach bool) (string, string, error) {
	var progressOutput strings.Builder

	// 步骤跟踪
	step := 1
	totalSteps := 5 // 总共5个步骤：配置、创建、验证、启动(可选)、完成

	// 步骤1: 配置
	progressOutput.WriteString(fmt.Sprintf("[%d/%d] 准备容器配置...\n", step, totalSteps))
	step++

	// 步骤2: 创建容器
	progressOutput.WriteString(fmt.Sprintf("[%d/%d] 创建容器...\n", step, totalSteps))
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", progressOutput.String(), fmt.Errorf("创建容器失败: %v", err)
	}
	step++

	// 步骤3: 验证
	progressOutput.WriteString(fmt.Sprintf("[%d/%d] 验证容器...\n", step, totalSteps))
	step++

	// 如果需要启动容器
	if detach {
		// 步骤4: 启动容器
		progressOutput.WriteString(fmt.Sprintf("[%d/%d] 启动容器...\n", step, totalSteps))

		err = cli.ContainerStart(ctx, resp.ID, container.StartOptions{})
		if err != nil {
			return resp.ID, progressOutput.String(), fmt.Errorf("启动容器失败: %v", err)
		}

		// 等待一下，给容器启动一些时间
		time.Sleep(1 * time.Second)

		// 检查容器状态
		containerInfo, err := cli.ContainerInspect(ctx, resp.ID)
		if err == nil && containerInfo.State.Running {
			progressOutput.WriteString("容器成功启动并正在运行!\n")
		}

		step++
	}

	// 步骤5: 完成
	progressOutput.WriteString(fmt.Sprintf("[%d/%d] 操作完成!\n", totalSteps, totalSteps))

	return resp.ID, progressOutput.String(), nil
}

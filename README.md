# Docker & Kubernetes MCP Server

这是一个基于MCP（Modular Communication Protocol）协议的Docker和Kubernetes管理服务器，支持API密钥身份验证，使用Eino框架构建AI助手来管理容器和Kubernetes资源。

## 项目结构

```
mcp-docker/
├── client/               # 客户端代码目录
│   ├── cmd/              # 命令行工具目录
│   │   └── main.go       # 客户端入口，与MCP服务器交互
│   ├── pkg/              # 客户端核心包
│   └── .env              # 客户端环境变量配置
├── server/               # 服务端代码目录
│   ├── main.go           # 服务端入口，MCP服务器实现
│   ├── auth/             # 认证相关代码
│   ├── docker/           # Docker相关工具实现
│   ├── k8s/              # Kubernetes相关工具实现
│   └── .env              # 服务端环境变量配置
├── .env                  # 根目录环境变量配置（可共享）
├── go.mod                # Go模块定义
└── go.sum                # 依赖版本锁定
```

## 功能特性

- Docker容器管理（创建、启动、停止、删除等）
- Docker镜像管理（列出、拉取、删除）
- Docker卷和网络管理
- Kubernetes资源管理（Pod、Deployment、Service、Namespace等）
- 支持API密钥身份验证
- 基于Eino框架的智能助手交互
- 支持从.env文件加载配置

## 快速开始

### 前置条件

- Go 1.21+
- 本地Docker服务
- （可选）Kubernetes集群或单节点环境

### 配置与启动

1. **克隆项目并安装依赖**：
   ```bash
   git clone https://github.com/yourusername/mcp-docker.git
   cd mcp-docker
   go mod download
   ```

2. **配置环境变量**：
   复制示例配置文件并编辑
   ```bash
   cp .env.example .env
   # 编辑.env文件，设置API_KEY和其他配置
   ```

3. **启动服务端**：
   ```bash
   go run server/main.go
   ```

4. **启动客户端**：
   ```bash
   go run client/cmd/main.go
   ```

## 环境变量配置

### 共享配置

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `API_KEY` | API密钥，用于客户端和服务端认证 | - |

### 服务端配置

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `MCP_SERVER_ADDRESS` | 服务器监听地址 | `localhost:12345` |

### 客户端配置

| 变量名 | 描述 | 默认值 |
|--------|------|--------|
| `MCP_SERVER_URL` | 服务器URL | `http://localhost:12345/sse` |
| `OPENAI_API_KEY` | OpenAI API密钥 | - |
| `OPENAI_BASE_URL` | OpenAI API基础URL | `https://api.openai.com/v1` |
| `OPENAI_MODEL` | 使用的OpenAI模型 | `gpt-4` |

## 支持的工具

### Docker工具

| 工具名 | 描述 |
|--------|------|
| `list_containers` | 列出所有容器 |
| `start_container` | 启动已停止的容器 |
| `stop_container` | 停止指定的容器 |
| `create_container` | 创建并运行一个新容器 |
| `remove_container` | 删除指定的容器 |
| `restart_container` | 重启指定的容器 |
| `container_logs` | 查看容器日志 |
| `inspect_container` | 查看容器详细信息 |
| `container_status` | 快速检查容器的运行状态 |
| `list_images` | 列出所有镜像 |
| `remove_image` | 删除指定的镜像 |
| `pull_image` | 拉取指定的镜像 |
| `system_info` | 显示Docker系统信息 |
| `system_prune` | 清理未使用的Docker对象 |
| `list_volumes` | 列出所有卷 |
| `remove_volume` | 删除指定的卷 |
| `list_networks` | 列出所有网络 |
| `remove_network` | 删除指定的网络 |

### Kubernetes工具

| 工具名 | 描述 |
|--------|------|
| `list_pods` | 列出指定命名空间中的所有Pod |
| `describe_pod` | 查看Pod的详细信息 |
| `delete_pod` | 删除指定的Pod |
| `pod_logs` | 获取Pod的日志 |
| `list_deployments` | 列出指定命名空间中的所有Deployment |
| `describe_deployment` | 查看Deployment的详细信息 |
| `scale_deployment` | 调整Deployment的副本数 |
| `restart_deployment` | 重启Deployment的所有Pod |
| `list_services` | 列出指定命名空间中的所有Service |
| `describe_service` | 查看Service的详细信息 |
| `list_namespaces` | 列出所有命名空间 |
| `describe_namespace` | 查看命名空间的详细信息 |
| `create_namespace` | 创建新的命名空间 |
| `delete_namespace` | 删除指定的命名空间 |

## 客户端使用

客户端提供基于Eino框架的交互式AI助手，可以通过自然语言与Docker和Kubernetes进行交互。例如：

- "列出所有运行中的容器"
- "启动名为web的容器"
- "删除所有停止的容器"
- "拉取最新的nginx镜像"
- "查看default命名空间中的所有pod"

## 服务端架构

服务端基于MCP协议实现，主要组件包括：

1. **MCP服务器**: 处理客户端请求，分发到相应的工具处理函数
2. **认证中间件**: 验证API密钥
3. **工具集合**:
   - Docker相关工具
   - Kubernetes相关工具

## 排障指南

### 连接问题

如果遇到"connect ECONNREFUSED"错误：

1. **确认服务器正在运行**：检查服务器启动日志
2. **确认地址配置**：
   - 服务器端使用 `0.0.0.0:12345` 而不是 `localhost:12345`
   - 客户端使用 `http://127.0.0.1:12345/sse` 而不是 `http://localhost:12345/sse`
3. **确认API密钥**：服务器和客户端需要使用相同的API密钥
4. **检查防火墙设置**：确保端口未被阻止

### OpenAI API问题

如果遇到OpenAI API相关错误：

1. **确认API密钥**：检查OPENAI_API_KEY环境变量是否正确设置
2. **检查模型可用性**：确保配置的模型名称是有效的
3. **网络连接**：确保能够连接到OpenAI API服务器

## 开发指南

### 添加新工具

在server/main.go中，使用以下模式添加新工具：

```go
svr.AddTool(mcp.NewTool("tool_name",
    mcp.WithDescription("工具描述"),
    mcp.WithString("param_name",
        mcp.Required(),
        mcp.Description("参数描述"),
    ),
), handlerFunction)
```

然后实现对应的处理函数。

## 许可证

MIT 
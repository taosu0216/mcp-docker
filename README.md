# MCP-Docker - 云原生容器管理系统

![版本](https://img.shields.io/badge/版本-1.0.0-blue)
![Docker](https://img.shields.io/badge/Docker-支持-brightgreen)
![Kubernetes](https://img.shields.io/badge/Kubernetes-支持-brightgreen)
![语言](https://img.shields.io/badge/语言-Go_1.23+-orange)

## 项目简介

MCP-Docker 是一个基于 MCP (Modular Communication Protocol) 协议的云原生容器管理系统，提供了直观的命令行界面来管理 Docker 容器和 Kubernetes 资源。该系统采用客户端-服务器架构，实现了 AI 驱动的容器管理功能，简化了容器操作流程。

## 核心功能

### Docker 资源管理
- 容器管理：创建、启动、停止、重启、删除容器
- 镜像管理：拉取、查看、删除镜像
- 网络管理：查看、创建、删除网络
- 卷管理：查看、创建、删除卷
- 系统管理：查看系统信息、清理未使用资源

### Kubernetes 资源管理
- Pod 管理：查看、描述、删除 Pod 及日志查看
- Deployment 管理：查看、描述、伸缩、重启 Deployment
- Service 管理：查看、描述 Service
- 命名空间管理：查看、创建、删除命名空间

## 技术架构

### 客户端 (Client)
- 基于 Eino 框架构建的 AI 驱动交互界面
- 通过 MCP 协议与服务端通信
- 支持自然语言命令解析和执行
- 提供命令重试和错误恢复机制

### 服务端 (Server)
- 提供 RESTful API 和 SSE 接口
- 集成 Docker 和 Kubernetes API
- 实现丰富的 MCP 工具集
- 支持会话管理和健康检查

## 安装指南

### 环境要求
- Go 1.23 或更高版本
- Docker 引擎 (建议 24.0+)
- Kubernetes 集群 (可选)
- OpenAI API 密钥 (客户端需要)

### 使用 Docker Compose 部署

1. 克隆代码仓库
```bash
git clone https://github.com/taosu0216/mcp-docker.git
cd mcp-docker
```

2. 配置环境变量
```bash
cp .env.example .env
# 编辑 .env 文件，设置API密钥和其他配置
```

### 手动部署

1. 克隆代码仓库
```bash
git clone https://github.com/taosu0216/mcp-docker.git
cd mcp-docker
```

2. 配置环境变量
```bash
cp .env.example .env
# 编辑 .env 文件，设置API密钥和其他配置
```

3. 构建并启动服务端
```bash
cd server
go build
./server
```

4. 构建并启动客户端
```bash
cd client/cmd
go build
./client
```

## 使用指南

### 服务端
服务端启动后，将在配置的地址和端口上监听请求。默认地址为 `0.0.0.0:12345`。

### 客户端
客户端启动后，将通过自然语言交互方式提供容器管理功能。

```
==== 云原生容器管理客户端启动 ====
支持 Docker 和 Kubernetes 资源管理
使用服务器URL: http://127.0.0.1:12345/sse
正在连接MCP服务器...
MCP连接已建立，等待连接稳定...
正在获取MCP工具...
[系统] 第 1 次尝试获取工具...
[系统] 成功获取 40 个工具，初始化完成
客户端准备就绪，请输入您的命令 (输入'exit'退出):

You: 
```

### 示例命令

#### Docker 管理示例
```
查看所有容器
拉取最新的 nginx 镜像
启动一个新的 nginx 容器并映射80端口
查看容器日志
停止并删除容器
```

#### Kubernetes 管理示例
```
查看所有命名空间
获取 default 命名空间中的所有 Pod
查看 nginx-deployment 的详细信息
伸缩 nginx-deployment 到3个副本
查看 Pod 的日志
```

## 排障指南

### 常见问题

1. **连接服务器失败**
   - 检查服务器地址和端口是否正确
   - 确认服务器是否正常运行
   - 检查网络连接和防火墙设置

2. **API密钥不正确**
   - 确认 .env 文件中 API_KEY 设置正确
   - 确认客户端和服务端使用相同的 API 密钥

3. **工具获取失败**
   - 尝试使用"更新工具"命令手动刷新
   - 重启客户端和服务端
   - 检查网络延迟和连接稳定性

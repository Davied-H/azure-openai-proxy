# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

Azure OpenAI 代理服务，将 OpenAI 兼容的 API 请求路由到多个 Azure OpenAI 后端，提供负载均衡、健康检查和自动故障转移。

## 常用命令

### 构建
```bash
# 构建当前平台
./build.sh current

# 构建所有平台 (Linux/macOS/Windows)
./build.sh all

# 指定版本构建
VERSION=2.0.0 ./build.sh all

# 清理构建产物
./build.sh clean
```

### 运行
```bash
# 直接运行
./azure-openai-proxy --config config.yaml

# Docker 运行
docker-compose up -d
```

### 测试
```bash
# 运行嵌入测试
python test_embedding.py
```

## 架构

```
main.go                    # 入口点，路由注册，启动健康检查
├── config/config.go       # YAML 配置加载与验证
├── handlers/proxy.go      # 请求转发逻辑（chat/embeddings/responses）
├── middleware/
│   ├── auth.go           # API Key 认证（支持 Bearer/api-key/x-api-key）
│   └── logger.go         # 请求日志与 panic 恢复
└── loadbalancer/balancer.go  # 轮询负载均衡，健康追踪
```

### 请求流程

1. 认证中间件验证 API Key
2. Handler 从请求体提取 model 名称
3. LoadBalancer 返回健康后端列表（轮询顺序）
4. 请求转发到 Azure OpenAI 端点
5. 5xx 错误或失败时标记后端不健康，尝试下一个后端
6. 不健康后端 30 秒后自动恢复

### 关键设计

- **单例模式**: LoadBalancer 使用 sync.Once
- **原子操作**: 轮询计数器使用 atomic 保证并发安全
- **流式支持**: 4KB 缓冲区处理 SSE 响应
- **安全**: 常量时间 API Key 比较防止时序攻击

## 配置文件 (config.yaml)

```yaml
server:
  port: 3000

auth:
  enabled: true
  keys:
    - name: "default"
      key: "your-api-key"

models:
  gpt-4:
    backends:
      - endpoint: "https://xxx.openai.azure.com"
        api_key: "azure-key"
        deployment: "gpt-4"
        api_version: "2025-04-01-preview"

retry:
  max_attempts: 3
  timeout: 30s
```

## API 端点

| 端点 | 说明 |
|------|------|
| `GET /health` | 健康检查（无需认证） |
| `POST /v1/chat/completions` | Chat API |
| `POST /v1/embeddings` | Embeddings API |
| `POST /v1/responses` | Responses API |

## 技术栈

- Go 1.24.0
- Gin（HTTP 框架）
- Viper（配置管理）
- Zap（结构化日志）
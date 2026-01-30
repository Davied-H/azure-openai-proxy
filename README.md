# Azure OpenAI Proxy

将 OpenAI 兼容的 API 请求路由到多个 Azure OpenAI 后端，提供负载均衡、健康检查和自动故障转移。

## 特性

- **OpenAI 兼容 API**: 支持 `/v1/chat/completions`、`/v1/embeddings`、`/v1/responses` 端点
- **多后端负载均衡**: 轮询调度，自动分发请求到多个 Azure OpenAI 实例
- **自动故障转移**: 后端失败时自动切换，30 秒后自动恢复
- **健康检查**: 定时检测后端状态，标记不健康节点
- **API Key 认证**: 支持 Bearer Token、api-key、x-api-key 三种认证方式
- **流式响应**: 支持 SSE 流式输出
- **跨平台**: 支持 Linux、macOS、Windows

## 快速开始

### 1. 配置

从模板创建配置文件：

```bash
cp config.template.yaml config.yaml
```

然后编辑 `config.yaml`，填入实际的 Azure OpenAI 端点和密钥：

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
      - endpoint: "https://your-resource.openai.azure.com"
        api_key: "your-azure-api-key"
        deployment: "gpt-4"
        api_version: "2025-04-01-preview"
      - endpoint: "https://your-resource-2.openai.azure.com"
        api_key: "your-azure-api-key-2"
        deployment: "gpt-4"
        api_version: "2025-04-01-preview"

  text-embedding-ada-002:
    backends:
      - endpoint: "https://your-resource.openai.azure.com"
        api_key: "your-azure-api-key"
        deployment: "text-embedding-ada-002"
        api_version: "2023-05-15"

retry:
  max_attempts: 3
  timeout: 30s
```

### 2. 构建

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

### 3. 运行

```bash
# 直接运行
./azure-openai-proxy --config config.yaml

# Docker 运行
docker-compose up -d
```

## API 端点

| 端点 | 方法 | 说明 | 认证 |
|------|------|------|------|
| `/health` | GET | 健康检查 | 否 |
| `/v1/chat/completions` | POST | Chat API | 是 |
| `/v1/embeddings` | POST | Embeddings API | 是 |
| `/v1/responses` | POST | Responses API | 是 |

## 认证

启用认证后，请求需要携带有效的 API Key，支持以下三种方式：

```bash
# Bearer Token
curl -H "Authorization: Bearer your-api-key" ...

# api-key header
curl -H "api-key: your-api-key" ...

# x-api-key header
curl -H "x-api-key: your-api-key" ...
```

## 使用示例

### Chat Completions

```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

### Embeddings

```bash
curl http://localhost:3000/v1/embeddings \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-embedding-ada-002",
    "input": "Hello world"
  }'
```

## 架构

```
main.go                        # 入口点，路由注册，启动健康检查
├── config/config.go           # YAML 配置加载与验证
├── handlers/proxy.go          # 请求转发逻辑（chat/embeddings/responses）
├── middleware/
│   ├── auth.go               # API Key 认证
│   └── logger.go             # 请求日志与 panic 恢复
└── loadbalancer/balancer.go  # 轮询负载均衡，健康追踪
```

### 请求流程

1. 认证中间件验证 API Key
2. Handler 从请求体提取 model 名称
3. LoadBalancer 返回健康后端列表（轮询顺序）
4. 请求转发到 Azure OpenAI 端点
5. 5xx 错误或失败时标记后端不健康，尝试下一个后端
6. 不健康后端 30 秒后自动恢复

## 配置说明

### server

| 字段 | 类型 | 说明 |
|------|------|------|
| `port` | int | 服务端口，默认 3000 |

### auth

| 字段 | 类型 | 说明 |
|------|------|------|
| `enabled` | bool | 是否启用认证 |
| `keys` | array | API Key 列表 |
| `keys[].name` | string | Key 名称（用于日志） |
| `keys[].key` | string | API Key 值 |

### models

按模型名称配置后端池，每个模型可配置多个后端用于负载均衡。

| 字段 | 类型 | 说明 |
|------|------|------|
| `backends` | array | 后端列表 |
| `backends[].endpoint` | string | Azure OpenAI 端点 |
| `backends[].api_key` | string | Azure API Key |
| `backends[].deployment` | string | 部署名称 |
| `backends[].api_version` | string | API 版本 |

### retry

| 字段 | 类型 | 说明 |
|------|------|------|
| `max_attempts` | int | 最大重试次数 |
| `timeout` | duration | 请求超时时间 |

## 技术栈

- Go 1.24.0
- [Gin](https://github.com/gin-gonic/gin) - HTTP 框架
- [Viper](https://github.com/spf13/viper) - 配置管理
- [Zap](https://go.uber.org/zap) - 结构化日志

## License

MIT
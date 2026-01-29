package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"azure-openai-proxy/config"
	"azure-openai-proxy/loadbalancer"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const maxBodySize = 10 * 1024 * 1024 // 10MB

type ProxyHandler struct {
	lb     *loadbalancer.LoadBalancer
	cfg    *config.Config
	logger *zap.Logger
	client *http.Client
}

func NewProxyHandler(lb *loadbalancer.LoadBalancer, cfg *config.Config, logger *zap.Logger) *ProxyHandler {
	return &ProxyHandler{
		lb:     lb,
		cfg:    cfg,
		logger: logger,
		client: &http.Client{
			Timeout: cfg.Retry.Timeout,
		},
	}
}

// 从请求体中提取模型名称
func extractModel(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return ""
	}
	return req.Model
}

// Azure OpenAI 不支持的参数列表
var unsupportedParams = []string{
	"chat_template_kwargs",
	"enable_thinking",
}

// transformRequestBody 转换请求体中的参数
// 1. 将 max_tokens 转换为 max_completion_tokens（新版 Azure OpenAI API 要求）
// 2. 移除 Azure OpenAI 不支持的参数
func transformRequestBody(body []byte, logger *zap.Logger) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	modified := false

	// 如果存在 max_tokens，转换为 max_completion_tokens
	if maxTokens, exists := data["max_tokens"]; exists {
		if _, hasNewParam := data["max_completion_tokens"]; !hasNewParam {
			data["max_completion_tokens"] = maxTokens
			delete(data, "max_tokens")
			logger.Info("transformed max_tokens to max_completion_tokens",
				zap.Any("value", maxTokens))
			modified = true
		}
	}

	// 移除不支持的参数
	for _, param := range unsupportedParams {
		if _, exists := data[param]; exists {
			delete(data, param)
			logger.Info("removed unsupported parameter", zap.String("param", param))
			modified = true
		}
	}

	if !modified {
		return body
	}

	newBody, err := json.Marshal(data)
	if err != nil {
		return body
	}
	return newBody
}

// HandleEmbeddings 处理 Embedding 请求
func (h *ProxyHandler) HandleEmbeddings(c *gin.Context) {
	h.handleOpenAIRequest(c, "embeddings")
}

// HandleChatCompletions 处理 Chat Completions 请求
func (h *ProxyHandler) HandleChatCompletions(c *gin.Context) {
	h.handleOpenAIRequest(c, "chat/completions")
}

// HandleResponses 处理 Responses API 请求
func (h *ProxyHandler) HandleResponses(c *gin.Context) {
	h.handleOpenAIRequest(c, "responses")
}

// handleOpenAIRequest 处理 OpenAI 兼容格式的请求
func (h *ProxyHandler) handleOpenAIRequest(c *gin.Context, apiType string) {
	h.logger.Info("received request",
		zap.String("api_type", apiType),
		zap.String("method", c.Request.Method),
		zap.String("path", c.Request.URL.Path),
	)

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodySize))
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}
	if int64(len(body)) >= maxBodySize {
		h.logger.Error("request body too large", zap.Int64("size", int64(len(body))))
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		return
	}

	h.logger.Debug("request body", zap.String("body", string(body)))

	model := extractModel(body)
	if model == "" {
		h.logger.Error("model field is missing from request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "model field is required"})
		return
	}

	h.logger.Info("extracted model", zap.String("model", model))

	if !h.lb.HasModel(model) {
		h.logger.Error("model not configured", zap.String("model", model))
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("model %s is not configured", model)})
		return
	}

	// 转换请求参数（如 max_tokens -> max_completion_tokens）
	body = transformRequestBody(body, h.logger)

	h.proxyWithModel(c, model, body, apiType)
}

func (h *ProxyHandler) proxyWithModel(c *gin.Context, model string, body []byte, apiType string) {
	h.logger.Info("proxyWithModel called",
		zap.String("model", model),
		zap.String("api_type", apiType),
	)

	backends := h.lb.GetAllBackends(model)
	if len(backends) == 0 {
		h.logger.Error("no backends available for model", zap.String("model", model))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no backends available"})
		return
	}

	h.logger.Info("found backends", zap.Int("count", len(backends)))

	var lastErr error
	maxAttempts := h.cfg.Retry.MaxAttempts
	if maxAttempts > len(backends) {
		maxAttempts = len(backends)
	}

	for i := 0; i < maxAttempts; i++ {
		// 检查 context 是否已取消
		select {
		case <-c.Request.Context().Done():
			h.logger.Info("request cancelled by client")
			return
		default:
		}

		backend := backends[i%len(backends)]

		// 从配置获取 api_version，如果未配置则使用默认值
		apiVersion := backend.Backend.APIVersion
		if apiVersion == "" {
			apiVersion = "2024-02-01"
		}

		// 构建目标 URL
		var targetURL string
		if apiType == "responses" {
			targetURL = fmt.Sprintf("%s/openai/responses?api-version=%s",
				strings.TrimSuffix(backend.Backend.Endpoint, "/"), apiVersion)
		} else {
			deploymentName := backend.Backend.Deployment
			targetURL = fmt.Sprintf("%s/openai/deployments/%s/%s?api-version=%s",
				strings.TrimSuffix(backend.Backend.Endpoint, "/"), deploymentName, apiType, apiVersion)
		}

		h.logger.Info("proxying request",
			zap.String("model", model),
			zap.String("target_url", targetURL),
			zap.String("api_version", apiVersion),
			zap.Int("attempt", i+1),
		)

		req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, bytes.NewBuffer(body))
		if err != nil {
			h.logger.Error("failed to create request", zap.Error(err))
			lastErr = err
			continue
		}

		// 复制请求头
		for key, values := range c.Request.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		req.Header.Set("api-key", backend.Backend.APIKey)
		req.Header.Set("Content-Type", "application/json")

		h.logger.Info("sending request to backend")
		resp, err := h.client.Do(req)
		if err != nil {
			h.logger.Warn("backend request failed",
				zap.String("target_url", targetURL),
				zap.Error(err),
			)
			h.lb.MarkUnhealthy(model, backend)
			lastErr = err
			continue
		}

		h.logger.Info("received response from backend",
			zap.Int("status_code", resp.StatusCode),
			zap.String("content_type", resp.Header.Get("Content-Type")),
		)

		// 检查响应状态码
		if resp.StatusCode >= 500 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			h.logger.Warn("backend returned error",
				zap.String("target_url", targetURL),
				zap.Int("status", resp.StatusCode),
				zap.String("body", string(respBody)),
			)
			h.lb.MarkUnhealthy(model, backend)
			lastErr = fmt.Errorf("backend returned status %d", resp.StatusCode)
			continue
		}

		// 成功，标记为健康
		h.lb.MarkHealthy(model, backend)

		// 检查是否为流式响应
		if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			h.logger.Info("handling stream response")
			h.handleStreamResponse(c, resp)
			return
		}

		// 非流式响应
		h.logger.Info("handling normal response")
		h.handleNormalResponse(c, resp)
		return
	}

	h.logger.Error("all backends failed",
		zap.String("model", model),
		zap.Error(lastErr),
	)
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error": "all backends failed",
		"detail": lastErr.Error(),
	})
}

func (h *ProxyHandler) handleStreamResponse(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	c.Stream(func(w io.Writer) bool {
		buf := make([]byte, 4096)
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				h.logger.Warn("failed to write stream response", zap.Error(writeErr))
				return false
			}
			c.Writer.Flush()
		}
		if err != nil && err != io.EOF {
			h.logger.Warn("error reading stream", zap.Error(err))
		}
		return err == nil
	})
}

func (h *ProxyHandler) handleNormalResponse(c *gin.Context, resp *http.Response) {
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read response"})
		return
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// HandleHealth 健康检查接口
func (h *ProxyHandler) HandleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

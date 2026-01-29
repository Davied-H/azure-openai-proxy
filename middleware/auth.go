package middleware

import (
	"net/http"
	"strings"

	"azure-openai-proxy/config"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ContextKeyAPIKeyName 用于在 context 中存储 API Key 名称的键
const ContextKeyAPIKeyName = "api_key_name"

// Auth 返回认证中间件
func Auth(cfg *config.Config, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果未启用认证，直接放行
		if !cfg.IsAuthEnabled() {
			c.Next()
			return
		}

		// 提取 API Key
		apiKey := extractAPIKey(c)
		if apiKey == "" {
			logger.Warn("missing api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("ip", c.ClientIP()),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing API key. Please include your API key in the Authorization header using Bearer scheme, or in the api-key/x-api-key header.",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			return
		}

		// 验证 API Key
		keyName, valid := cfg.ValidateAPIKey(apiKey)
		if !valid {
			// 日志中只记录 key 的前缀，避免泄露完整 key
			maskedKey := maskAPIKey(apiKey)
			logger.Warn("invalid api key",
				zap.String("path", c.Request.URL.Path),
				zap.String("ip", c.ClientIP()),
				zap.String("masked_key", maskedKey),
			)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Invalid API key provided.",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
			return
		}

		// 将 key 名称存入 context 供日志使用
		c.Set(ContextKeyAPIKeyName, keyName)
		c.Next()
	}
}

// extractAPIKey 从请求中提取 API Key
// 支持以下格式：
// 1. Authorization: Bearer <key>
// 2. api-key: <key>
// 3. x-api-key: <key>
func extractAPIKey(c *gin.Context) string {
	// 1. 优先检查 Authorization header (Bearer Token)
	auth := c.GetHeader("Authorization")
	if auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// 2. 检查 api-key header
	if apiKey := c.GetHeader("api-key"); apiKey != "" {
		return apiKey
	}

	// 3. 检查 x-api-key header
	if apiKey := c.GetHeader("x-api-key"); apiKey != "" {
		return apiKey
	}

	return ""
}

// maskAPIKey 遮蔽 API Key，只显示前 8 个字符
func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:8] + "***"
}

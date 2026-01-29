package config

import (
	"crypto/subtle"
	"time"

	"github.com/spf13/viper"
)

type Backend struct {
	Endpoint   string `mapstructure:"endpoint"`
	APIKey     string `mapstructure:"api_key"`
	Deployment string `mapstructure:"deployment"`
	APIVersion string `mapstructure:"api_version"`
}

type ModelConfig struct {
	Backends []Backend `mapstructure:"backends"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type RetryConfig struct {
	MaxAttempts int           `mapstructure:"max_attempts"`
	Timeout     time.Duration `mapstructure:"timeout"`
}

// APIKeyConfig 单个 API Key 配置
type APIKeyConfig struct {
	Name string `mapstructure:"name"`
	Key  string `mapstructure:"key"`
}

// AuthConfig 认证配置
type AuthConfig struct {
	Enabled bool           `mapstructure:"enabled"`
	Keys    []APIKeyConfig `mapstructure:"keys"`
}

type Config struct {
	Server ServerConfig           `mapstructure:"server"`
	Models map[string]ModelConfig `mapstructure:"models"`
	Retry  RetryConfig            `mapstructure:"retry"`
	Auth   AuthConfig             `mapstructure:"auth"`
}

var AppConfig *Config

func Load(configPath string) error {
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	// 设置默认值
	v.SetDefault("server::port", 8080)
	v.SetDefault("retry::max_attempts", 3)
	v.SetDefault("retry::timeout", "30s")

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	AppConfig = &Config{}
	if err := v.Unmarshal(AppConfig); err != nil {
		return err
	}

	return nil
}

// GetBackendsForModel 获取指定模型的后端列表
func (c *Config) GetBackendsForModel(model string) []Backend {
	if modelConfig, ok := c.Models[model]; ok {
		return modelConfig.Backends
	}
	return nil
}

// IsAuthEnabled 检查是否启用认证
func (c *Config) IsAuthEnabled() bool {
	return c.Auth.Enabled && len(c.Auth.Keys) > 0
}

// ValidateAPIKey 验证 API Key，返回 key 名称和是否有效
// 使用常量时间比较防止时序攻击
func (c *Config) ValidateAPIKey(key string) (string, bool) {
	if !c.IsAuthEnabled() {
		return "", true
	}

	for _, k := range c.Auth.Keys {
		if subtle.ConstantTimeCompare([]byte(key), []byte(k.Key)) == 1 {
			return k.Name, true
		}
	}
	return "", false
}

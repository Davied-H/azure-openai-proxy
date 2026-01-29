package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"azure-openai-proxy/config"
	"azure-openai-proxy/handlers"
	"azure-openai-proxy/loadbalancer"
	"azure-openai-proxy/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 初始化日志
	logConfig := zap.NewProductionConfig()
	logConfig.EncoderConfig.TimeKey = "timestamp"
	logConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	logger, err := logConfig.Build()
	if err != nil {
		log.Fatalf("初始化日志失败: %v", err)
	}
	defer logger.Sync()

	// 加载配置
	if err := config.Load(*configPath); err != nil {
		logger.Fatal("加载配置失败", zap.Error(err))
	}

	// 打印加载的模型列表
	var modelNames []string
	for name := range config.AppConfig.Models {
		modelNames = append(modelNames, name)
	}
	logger.Info("配置加载成功",
		zap.Int("models_count", len(config.AppConfig.Models)),
		zap.Strings("models", modelNames),
		zap.Int("port", config.AppConfig.Server.Port),
		zap.Bool("auth_enabled", config.AppConfig.IsAuthEnabled()),
	)

	// 初始化负载均衡器
	lb := loadbalancer.GetInstance()
	lb.Init(config.AppConfig)
	lb.StartHealthCheck(10 * time.Second)
	logger.Info("负载均衡器初始化成功")

	// 创建处理器
	proxyHandler := handlers.NewProxyHandler(lb, config.AppConfig, logger)

	// 设置 Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(middleware.Logger(logger))
	router.Use(middleware.Recovery(logger))

	// 路由
	router.GET("/health", proxyHandler.HandleHealth)

	// OpenAI 兼容 API 路由 (/v1/...)
	v1 := router.Group("/v1")
	v1.Use(middleware.Auth(config.AppConfig, logger))
	{
		v1.POST("/chat/completions", proxyHandler.HandleChatCompletions)
		v1.POST("/embeddings", proxyHandler.HandleEmbeddings)
		v1.POST("/responses", proxyHandler.HandleResponses)
	}

	// 启动服务
	addr := fmt.Sprintf(":%d", config.AppConfig.Server.Port)
	logger.Info("服务启动", zap.String("addr", addr))
	if err := router.Run(addr); err != nil {
		logger.Fatal("服务启动失败", zap.Error(err))
	}
}

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"modelgate/internal/anthropic"
	"modelgate/internal/apikey"
	"modelgate/internal/auth"
	"modelgate/internal/cache"
	"modelgate/internal/concurrency"
	"modelgate/internal/config"
	"modelgate/internal/db"
	"modelgate/internal/logger"
	"modelgate/internal/middleware"
	"modelgate/internal/model"
	"modelgate/internal/models"
	"modelgate/internal/proxy"
	"modelgate/internal/quota"
	"modelgate/internal/static"
	"modelgate/internal/usage"
	"modelgate/internal/user"
)

func main() {
	// 加载配置
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 设置 gin 模式
	gin.SetMode(cfg.Server.Mode)

	// 创建 ConfigManager
	cfgManager := config.NewManager(cfg, "config.yaml")
	log.Println("ConfigManager initialized with hot-reload support")

	// 连接数据库
	database, err := db.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// 执行数据库迁移
	if err := database.Migrate(); err != nil {
		log.Printf("Migration warning: %v", err)
	}
	log.Println("Database migrated (quota usage tables only)")

	// 初始化日志记录器
	userLogger := logger.NewUserLogger(cfg.Logs.Path, cfg.Logs.RetentionDays)
	defer userLogger.Close()

	// 初始化本地缓存
	localCache := cache.New()
	defer localCache.Stop()
	log.Println("Local cache initialized")

	// 初始化存储层
	userStore := models.NewUserStore(database.DB)
	apiKeyStore := models.NewAPIKeyStore(database.DB)
	// 新的 ConfigManager-based 存储
	modelStore := models.NewModelStore(cfgManager)
	backendStore := models.NewBackendStore(cfgManager)
	quotaStore := models.NewQuotaStore(cfgManager, database.DB) // 需要 CM 和 DB

	// 初始化 JWT 管理器
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// 初始化服务层
	apiKeyService := apikey.NewService(apiKeyStore, userStore, localCache)
	quotaService := quota.NewService(quotaStore, modelStore)
	usageService := usage.NewService(userLogger)

	// 初始化负载均衡器
	lb := proxy.NewRoundRobinBalancer()

	// 从 ConfigManager 加载模型和后端配置
	lb.ReloadConfig(cfgManager.GetModels())
	log.Printf("Loaded %d models from config", len(cfgManager.GetModels()))

	// 启动健康检查（每 30 秒检查一次）
	lb.StartHealthCheck(30 * time.Second)
	log.Printf("Health check started with 30s interval")

	// 初始化代理
	proxyInstance := proxy.NewProxy(lb, quotaService, usageService, modelStore, backendStore, userStore)

	// 设置配置变更监听 - 热重载支持
	configChanges := cfgManager.Subscribe()
	go func() {
		for event := range configChanges {
			switch event.Type {
			case "models", "all":
				log.Println("Config reload detected: updating load balancer")
				lb.ReloadConfig(cfgManager.GetModels())
				log.Printf("Load balancer updated with %d models", len(cfgManager.GetModels()))
			}
		}
	}()
	log.Println("Config hot-reload listener started")

	// 初始化并发限制器
	var concurrencyLimiter *concurrency.Limiter
	if cfg.Concurrency.GlobalLimit > 0 || cfg.Concurrency.UserLimit > 0 {
		concurrencyLimiter = concurrency.NewLimiter(cfg.Concurrency.GlobalLimit, cfg.Concurrency.UserLimit)
		log.Printf("Concurrency limiter enabled: global=%d, user=%d",
			cfg.Concurrency.GlobalLimit, cfg.Concurrency.UserLimit)
	}

	// 注意：配额策略现在直接从 config.yaml 读取，无需初始化到数据库
	log.Printf("Loaded %d quota policies from config", len(cfgManager.GetPolicies()))

	// 创建默认管理员
	createDefaultAdmin(userStore, cfg.Admin.DefaultEmail, cfg.Admin.DefaultPassword)

	// 设置路由
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// 静态文件服务（嵌入的前端页面）
	r.Use(static.Serve())

	// API v1 路由组
	api := r.Group("/api/v1")

	// 注册各模块路由
	userHandler := user.NewHandler(user.NewHandlerParams{
		Store:        userStore,
		JWTManager:   jwtManager,
		QuotaService: quotaService,
		QuotaStore:   quotaStore,
		Cache:        localCache,
		SSOConfig:    cfg.SSO,
		FeedbackURL:  cfg.Frontend.FeedbackURL,
		DevManualURL: cfg.Frontend.DevManualURL,
	})
	userHandler.RegisterRoutes(api)

	apiKeyHandler := apikey.NewHandler(apiKeyService, userStore)
	apiKeyHandler.RegisterRoutes(api, jwtManager)

	modelHandler := model.NewHandler(modelStore, backendStore, lb, userStore)
	modelHandler.RegisterRoutes(api, jwtManager)

	adminHandler := model.NewAdminHandler(quotaStore, userStore)
	adminHandler.RegisterRoutes(api, jwtManager)

	// 并发状态管理 API（管理员）
	if concurrencyLimiter != nil {
		adminAPI := api.Group("/admin")
		adminAPI.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, userStore))
		adminAPI.Use(middleware.AdminRequired())
		{
			adminAPI.GET("/concurrency/stats", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"data": concurrencyLimiter.GetStats()})
			})
			adminAPI.GET("/cache/stats", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"data": localCache.Stats()})
			})
		}
	}

	// OpenAI 兼容代理接口
	proxyHandler := apikey.NewProxyHandler(apiKeyService, proxyInstance, jwtManager, userStore)
	proxyHandler.RegisterRoutes(r, concurrencyLimiter)

	// Anthropic 兼容代理接口
	anthropicHandler := anthropic.NewHandler(proxyInstance)
	anthropicHandler.RegisterRoutes(r, proxyHandler.AuthMiddleware())

	log.Println("Anthropic API support enabled at /v1/messages")

	// 启动清理任务
	go cleanupTask(usageService)

	// 启动服务器
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("Server starting on %s", addr)

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := r.Run(addr); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	// 刷新使用记录
	usageService.Flush()

	log.Println("Server stopped")
}

func createDefaultAdmin(store *models.UserStore, email, password string) {
	if email == "" {
		return
	}

	// 检查是否已存在
	existing, err := store.GetByEmail(email)
	if err != nil {
		log.Printf("Failed to check existing admin: %v", err)
		return
	}
	if existing != nil {
		return
	}

	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return
	}

	admin := &models.User{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         "Administrator",
		Role:         models.RoleAdmin,
		QuotaPolicy:  "vip",
		Enabled:      true,
	}

	if err := store.Create(admin); err != nil {
		log.Printf("Failed to create default admin: %v", err)
		return
	}

	log.Printf("Created default admin: %s", email)
}

func cleanupTask(usageService *usage.Service) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Running cleanup task...")
		if err := usageService.CleanupOldRecords(); err != nil {
			log.Printf("Cleanup error: %v", err)
		}
	}
}

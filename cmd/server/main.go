package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"modelgate/internal/admin"
	"modelgate/internal/anthropic"
	"modelgate/internal/apikey"
	"modelgate/internal/auth"
	"modelgate/internal/cache"
	"modelgate/internal/concurrency"
	"modelgate/internal/config"
	"modelgate/internal/dashboard"
	"modelgate/internal/db"
	"modelgate/internal/entity"
	"modelgate/internal/logger"
	"modelgate/internal/middleware"
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

	// 初始化全局日志（根据运行模式设置日志级别和格式）
	development := cfg.Server.Mode == "debug"
	if err := logger.InitLogger(development); err != nil {
		log.Printf("Failed to initialize logger: %v", err)
	}

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
	userStore := entity.NewUserStore(database.DB)
	apiKeyStore := entity.NewAPIKeyStore(database.DB)
	// 新的 ConfigManager-based 存储
	modelStore := entity.NewModelStore(cfgManager)
	backendStore := entity.NewBackendStore(cfgManager)
	quotaStore := entity.NewQuotaStore(cfgManager, database.DB) // 需要 CM 和 DB

	// 初始化 JWT 管理器
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// 初始化服务层
	apiKeyService := apikey.NewService(apiKeyStore, userStore, localCache)
	dashboardService := dashboard.NewService(database.DB)
	quotaService := quota.NewService(quotaStore, modelStore, dashboardService)
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
	proxyInstance := proxy.NewProxy(lb, quotaService, usageService, modelStore, backendStore, userStore, cfg.DefaultModel)

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
		UsageService: usageService,
		Cache:        localCache,
		SSOConfig:    cfg.SSO,
		FeedbackURL:  cfg.Frontend.FeedbackURL,
		DevManualURL: cfg.Frontend.DevManualURL,
	})
	userHandler.RegisterRoutes(api)

	apiKeyHandler := apikey.NewHandler(apiKeyService, userStore)
	apiKeyHandler.RegisterRoutes(api, jwtManager)

	// Register admin handlers (admin endpoints)
	adminModelHandler := admin.NewModelHandler(modelStore, backendStore, lb, userStore)
	adminModelHandler.RegisterRoutes(api, jwtManager)

	adminPolicyHandler := admin.NewPolicyHandler(quotaStore, userStore)
	adminPolicyHandler.RegisterRoutes(api, jwtManager)

	// 注册 Dashboard 路由（需要用户认证，但不需要管理员权限）
	dashboardHandler := dashboard.NewHandler(dashboardService)
	dashboardAPI := api.Group("/dashboard")
	dashboardAPI.Use(middleware.AuthMiddlewareWithUserValidation(jwtManager, userStore))
	dashboardHandler.RegisterRoutes(dashboardAPI)
	log.Println("Dashboard routes registered at /api/v1/dashboard")

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
	proxyHandler := apikey.NewProxyHandler(apiKeyService, proxyInstance, jwtManager, userStore, usageService)
	proxyHandler.RegisterRoutes(r, concurrencyLimiter)

	// Anthropic 兼容代理接口
	anthropicHandler := anthropic.NewHandler(proxyInstance, usageService)
	anthropicHandler.RegisterRoutes(r, proxyHandler.AuthMiddleware())

	log.Println("Anthropic API support enabled at /v1/messages")

	// 启动清理任务
	go cleanupTask(usageService)

	// 创建 HTTP 服务器
	srv := &http.Server{
		Addr:           fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:        r,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	log.Printf("Server starting on %s (timeouts: read=%v, write=%v, idle=%v)",
		srv.Addr, cfg.Server.ReadTimeout, cfg.Server.WriteTimeout, cfg.Server.IdleTimeout)

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// 在 goroutine 中启动服务器
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-quit
	log.Println("Shutting down server...")

	// 创建带超时的上下文用于优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// 优雅关闭服务器
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	// 刷新使用记录
	usageService.Flush()

	log.Println("Server stopped")
}

func createDefaultAdmin(store *entity.UserStore, email, password string) {
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

	admin := &entity.User{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         "Administrator",
		Role:         entity.RoleAdmin,
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

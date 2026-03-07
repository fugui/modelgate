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
	modelStore := models.NewModelStore(database.DB)
	backendStore := models.NewBackendStore(database.DB)
	quotaStore := models.NewQuotaStore(database.DB)

	// 初始化 JWT 管理器
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// 初始化服务层
	apiKeyService := apikey.NewService(apiKeyStore, userStore, localCache)
	quotaService := quota.NewService(quotaStore, modelStore)
	usageService := usage.NewService(userLogger)

	// 初始化负载均衡器
	lb := proxy.NewRoundRobinBalancer()

	// 从数据库加载模型和后端配置
	modelList, err := modelStore.List()
	if err != nil {
		log.Fatalf("Failed to load models: %v", err)
	}

	// 如果数据库中有模型，加载它们及其后端
	if len(modelList) > 0 {
		for _, m := range modelList {
			// 加载该模型的所有后端
			backends, err := backendStore.ListByModel(m.ID)
			if err != nil {
				log.Printf("Failed to load backends for model %s: %v", m.ID, err)
				continue
			}

			for _, b := range backends {
				lb.AddBackend(m.ID, proxy.Backend{
					ID:        b.ID,
					URL:       b.BaseURL,
					Weight:    b.Weight,
					ModelName: b.ModelName,
					APIKey:    b.APIKey,
				})
				log.Printf("Loaded backend: %s -> %s (model: %s)", m.ID, b.BaseURL, b.ModelName)
			}
		}
	} else {
		// 从配置文件加载
		for _, m := range cfg.Models {
			// 创建模型
			model := &models.Model{
				ID:          m.ID,
				Name:        m.Name,
				Description: m.Description,
				Enabled:     m.Enabled,
				ModelParams: m.ModelParams,
			}
			if err := modelStore.Create(model); err != nil {
				log.Printf("Failed to create model %s: %v", m.ID, err)
				continue
			}
			log.Printf("Created model: %s", m.ID)

			// 创建后端
			for _, b := range m.Backends {
				backend := &models.Backend{
					ID:        b.ID,
					ModelID:   m.ID,
					Name:      b.Name,
					BaseURL:   b.BaseURL,
					APIKey:    b.APIKey,
					ModelName: b.ModelName,
					Weight:    b.Weight,
					Region:    b.Region,
					Enabled:   b.Enabled,
				}
				if backend.Weight == 0 {
					backend.Weight = 1
				}
				if err := backendStore.Create(backend); err != nil {
					log.Printf("Failed to create backend %s: %v", b.ID, err)
					continue
				}

				// 添加到负载均衡器
				lb.AddBackend(m.ID, proxy.Backend{
					ID:        b.ID,
					URL:       b.BaseURL,
					Weight:    b.Weight,
					ModelName: b.ModelName,
					APIKey:    b.APIKey,
				})
				log.Printf("Loaded backend from config: %s -> %s (model: %s)", m.ID, b.BaseURL, b.ModelName)
			}
		}
	}

	// 启动健康检查（每 30 秒检查一次）
	lb.StartHealthCheck(30 * time.Second)
	log.Printf("Health check started with 30s interval")

	// 初始化代理
	proxyInstance := proxy.NewProxy(lb, quotaService, usageService, modelStore, backendStore, userStore)

	// 初始化并发限制器
	var concurrencyLimiter *concurrency.Limiter
	if cfg.Concurrency.GlobalLimit > 0 || cfg.Concurrency.UserLimit > 0 {
		concurrencyLimiter = concurrency.NewLimiter(cfg.Concurrency.GlobalLimit, cfg.Concurrency.UserLimit)
		log.Printf("Concurrency limiter enabled: global=%d, user=%d",
			cfg.Concurrency.GlobalLimit, cfg.Concurrency.UserLimit)
	}

	// 初始化配额策略
	initQuotaPolicies(quotaStore, cfg.Policies)

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

func initQuotaPolicies(store *models.QuotaStore, policies []config.PolicyConfig) {
	if len(policies) == 0 {
		return
	}

	for _, p := range policies {
		policy := &models.QuotaPolicy{
			Name:              p.Name,
			RateLimit:         p.RateLimit,
			RateLimitWindow:   p.RateLimitWindow,
			RequestQuotaDaily: p.RequestQuotaDaily,
			Models:            p.Models,
			Description:       p.Description,
		}
		if err := store.CreateOrUpdatePolicy(policy); err != nil {
			log.Printf("Failed to init quota policy %s: %v", p.Name, err)
		} else {
			log.Printf("Loaded quota policy: %s", p.Name)
		}
	}
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

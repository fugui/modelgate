package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"llmgate/internal/apikey"
	"llmgate/internal/auth"
	"llmgate/internal/config"
	"llmgate/internal/db"
	"llmgate/internal/logger"
	"llmgate/internal/model"
	"llmgate/internal/models"
	"llmgate/internal/proxy"
	"llmgate/internal/quota"
	"llmgate/internal/usage"
	"llmgate/internal/user"
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

	// 初始化存储层
	userStore := models.NewUserStore(database.DB)
	apiKeyStore := models.NewAPIKeyStore(database.DB)
	modelStore := models.NewModelStore(database.DB)
	quotaStore := models.NewQuotaStore(database.DB)

	// 初始化 JWT 管理器
	jwtManager := auth.NewJWTManager(cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// 初始化服务层
	apiKeyService := apikey.NewService(apiKeyStore, userStore)
	quotaService := quota.NewService(quotaStore)
	usageService := usage.NewService(userLogger)

	// 初始化负载均衡器
	lb := proxy.NewRoundRobinBalancer()

	// 从数据库加载模型配置
	modelList, err := modelStore.ListEnabled()
	if err != nil {
		log.Fatalf("Failed to load models: %v", err)
	}

	for _, m := range modelList {
		lb.AddBackend(m.ID, proxy.Backend{
			URL:    m.BackendURL,
			Weight: m.Weight,
		})
		log.Printf("Loaded model: %s -> %s", m.ID, m.BackendURL)
	}

	// 如果没有模型，从配置文件加载
	if len(modelList) == 0 {
		for _, m := range cfg.Models {
			if m.Enabled {
				lb.AddBackend(m.ID, proxy.Backend{
					URL:    m.Backend,
					Weight: m.Weight,
				})
				log.Printf("Loaded model from config: %s -> %s", m.ID, m.Backend)

				// 保存到数据库
				_ = modelStore.Create(&models.Model{
					ID:          m.ID,
					Name:        m.Name,
					BackendURL:  m.Backend,
					Enabled:     m.Enabled,
					Weight:      m.Weight,
					Description: "",
				})
			}
		}
	}

	// 初始化代理
	proxyInstance := proxy.NewProxy(lb, quotaService, usageService, modelStore)

	// 创建默认管理员
	createDefaultAdmin(userStore, cfg.Admin.DefaultEmail, cfg.Admin.DefaultPassword)

	// 设置路由
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// API v1 路由组
	api := r.Group("/api/v1")

	// 注册各模块路由
	userHandler := user.NewHandler(userStore, jwtManager)
	userHandler.RegisterRoutes(api)

	apiKeyHandler := apikey.NewHandler(apiKeyService)
	apiKeyHandler.RegisterRoutes(api, jwtManager)

	modelHandler := model.NewHandler(modelStore)
	modelHandler.RegisterRoutes(api, jwtManager)

	adminHandler := model.NewAdminHandler(quotaStore)
	adminHandler.RegisterRoutes(api, jwtManager)

	// OpenAI 兼容代理接口
	proxyHandler := apikey.NewProxyHandler(apiKeyService, proxyInstance)
	proxyHandler.RegisterRoutes(r)

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

package web

import (
	"net/http"
	"strings"

	"service-controller-notebookum/internal/config"
	"service-controller-notebookum/internal/core/resilience"
	redisclient "service-controller-notebookum/internal/redis"
	"service-controller-notebookum/internal/web/handlers"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allowed := origin == "null" || // file:// pages send Origin: null
			origin == "https://api.universidad.localhost" ||
			strings.HasPrefix(origin, "http://localhost:")

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		} else if origin == "" {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Correlation-ID")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func NewRouter(cfg config.Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error", "An error occurred", middleware.CorrelationID(c))
	}))
	router.Use(corsMiddleware())
	router.Use(middleware.Correlation(cfg.CorrelationHeader))

	var rc *redisclient.Client
	if cfg.RedisHost != "" {
		rc = redisclient.New(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword)
	}

	registry := resilience.NewRegistry()
	health := handlers.NewHealthHandler(registry)
	users := handlers.NewUsersHandler(cfg)
	documentsHandler := handlers.NewDocumentsHandler(cfg, rc)
	summaries := handlers.NewSummariesHandler(cfg, rc)

	// Health / observability
	router.GET("/health", health.Health)
	router.GET("/ready", health.Ready)
	router.GET("/status/circuits", health.CircuitStatus)

	// Users — public endpoints (no auth required)
	router.POST("/api/v1/users", users.Create)
	router.POST("/api/v1/users/login", users.Login)
	router.POST("/api/v1/users/refresh", users.Refresh)
	router.GET("/api/v1/users/:id", users.Get)

	// Documents — protected
	router.POST("/api/v1/documento/upload", middleware.RequireAuth(), documentsHandler.Upload)
	router.GET("/api/v1/documents/:id/status", middleware.RequireAuth(), documentsHandler.Status)

	// Summaries — protected
	router.GET("/api/v1/summaries/:id", middleware.RequireAuth(), summaries.Get)
	router.POST("/api/v1/summaries/document", middleware.RequireAuth(), summaries.Create)

	router.NoRoute(func(c *gin.Context) {
		problem.Write(c, http.StatusNotFound, "Not Found", "Resource not found", middleware.CorrelationID(c))
	})

	return router
}

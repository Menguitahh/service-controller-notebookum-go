package web

import (
	"net/http"
	"strings"

	"service-controller-notebookum/internal/config"
	"service-controller-notebookum/internal/core/resilience"
	"service-controller-notebookum/internal/domain/documents"
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

	registry := resilience.NewRegistry()
	store := documents.NewStore()
	health := handlers.NewHealthHandler(registry)
	users := handlers.NewUsersHandler(cfg)
	documentsHandler := handlers.NewDocumentsHandler(store)
	summaries := handlers.NewSummariesHandler(store)

	router.GET("/health", health.Health)
	router.GET("/ready", health.Ready)
	router.GET("/status/circuits", health.CircuitStatus)
	router.POST("/api/v1/users", users.Create)
	router.GET("/api/v1/users/:id", users.Get)
	router.GET("/api/v1/documents/:id/status", middleware.RequireAuth(), documentsHandler.Status)
	router.POST("/api/v1/documento/upload", middleware.RequireAuth(), documentsHandler.Upload)
	router.GET("/api/v1/summaries/document/:id", middleware.RequireAuth(), summaries.Get)

	router.NoRoute(func(c *gin.Context) {
		problem.Write(c, http.StatusNotFound, "Not Found", "Resource not found", middleware.CorrelationID(c))
	})

	return router
}

package web

import (
	"net/http"

	"service-controller-notebookum/internal/config"
	"service-controller-notebookum/internal/core/resilience"
	"service-controller-notebookum/internal/domain/documents"
	"service-controller-notebookum/internal/web/handlers"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

func NewRouter(cfg config.Config) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.CustomRecovery(func(c *gin.Context, recovered any) {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error", "An error occurred", middleware.CorrelationID(c))
	}))
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
	router.POST("/api/v1/users", middleware.RequireAuth(), users.Create)
	router.GET("/api/v1/users/:id", middleware.RequireAuth(), users.Get)
	router.GET("/api/v1/documents/:id/status", middleware.RequireAuth(), documentsHandler.Status)
	router.POST("/api/v1/documento/upload", middleware.RequireAuth(), documentsHandler.Upload)
	router.GET("/api/v1/summaries/document/:id", middleware.RequireAuth(), summaries.Get)

	router.NoRoute(func(c *gin.Context) {
		problem.Write(c, http.StatusNotFound, "Not Found", "Resource not found", middleware.CorrelationID(c))
	})

	return router
}

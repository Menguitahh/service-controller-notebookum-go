package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"service-controller-notebookum/internal/config"
	redisclient "service-controller-notebookum/internal/redis"
	"service-controller-notebookum/internal/transport/upstream"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

type SummariesHandler struct {
	ai          *upstream.Client
	persistence *upstream.Client
	redis       *redisclient.Client
}

func NewSummariesHandler(cfg config.Config, rc *redisclient.Client) *SummariesHandler {
	return &SummariesHandler{
		ai:          upstream.New(cfg.AIURL, 120*time.Second),
		persistence: upstream.New(cfg.PersistenceURL, 10*time.Second),
		redis:       rc,
	}
}

// Get returns the summary for a document_id (UUID).
// Fast path: Redis key "summary:{document_id}".
// Slow path: Persistence GET /api/v1/summaries/document/{uuid}, result cached in Redis.
func (h *SummariesHandler) Get(c *gin.Context) {
	documentID := c.Param("id")

	// 1. Fast path — Redis
	if h.redis != nil {
		if cached, ok, _ := h.redis.Get("summary:" + documentID); ok && cached != "" {
			c.Data(http.StatusOK, "application/json", []byte(cached))
			return
		}
	}

	// 2. Health check — Persistence must be up before we try
	if !h.persistence.Ping("/actuator/health") {
		problem.Write(c, http.StatusServiceUnavailable, "Service Unavailable",
			"Persistence service is not available", middleware.CorrelationID(c))
		return
	}

	// 3. Slow path — Persistence
	status, body, _, err := h.persistence.Request(
		http.MethodGet,
		"/api/v1/summaries/document/"+documentID,
		nil,
		c.Request.Header.Clone(),
	)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway",
			"Persistence service unavailable", middleware.CorrelationID(c))
		return
	}

	// 4. Force-write to Redis on success so next call is served from cache
	if status == http.StatusOK && h.redis != nil {
		_ = h.redis.SetEX("summary:"+documentID, string(body), 3600)
	}

	c.Data(status, "application/json", body)
}

// Create proxies a summarization request to the AI service.
// Checks Redis first (already generated?), then forwards to AI.
// After AI responds, force-writes the result to Redis.
func (h *SummariesHandler) Create(c *gin.Context) {
	var payload struct {
		DocumentID string `json:"document_id"`
		Language   string `json:"language"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || payload.DocumentID == "" {
		problem.Write(c, http.StatusBadRequest, "Bad Request",
			"document_id is required", middleware.CorrelationID(c))
		return
	}

	// 1. Fast path — already in Redis (re-request after a previous generation)
	if h.redis != nil {
		if cached, ok, _ := h.redis.Get("summary:" + payload.DocumentID); ok && cached != "" {
			c.Data(http.StatusOK, "application/json", []byte(cached))
			return
		}
	}

	// 2. Health check — AI must be up
	if !h.ai.Ping("/health") {
		problem.Write(c, http.StatusServiceUnavailable, "Service Unavailable",
			"AI service is not available", middleware.CorrelationID(c))
		return
	}

	userID := c.GetString("user_id")

	reqBody, err := json.Marshal(map[string]any{
		"document_id": payload.DocumentID,
		"language":    payload.Language,
		"user_id":     userID,
	})
	if err != nil {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error",
			"Failed to build request", middleware.CorrelationID(c))
		return
	}

	headers := c.Request.Header.Clone()
	headers.Set("Content-Type", "application/json")

	// 3. Call AI
	status, body, _, err := h.ai.Request(http.MethodPost, "/api/v1/summaries/document", reqBody, headers)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway",
			"AI service unavailable", middleware.CorrelationID(c))
		return
	}

	// 4. Force-write to Redis (AI also writes, but we own the Controller key)
	if status == http.StatusOK && h.redis != nil {
		_ = h.redis.SetEX("summary:"+payload.DocumentID, string(body), 3600)
	}

	c.Data(status, "application/json", body)
}

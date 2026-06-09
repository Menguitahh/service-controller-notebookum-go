package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
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

	// 2. Slow path — Persistence
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

	if status == http.StatusOK && h.redis != nil {
		_ = h.redis.SetEX("summary:"+documentID, string(body), 3600)
	}

	c.Data(status, "application/json", body)
}

// Create generates a summary for an already-extracted document.
//
// Flow:
//  1. Redis fast path (previously generated)
//  2. Persistence check (already saved)
//  3. Call AI service (reads extraction from Redis, returns summary)
//  4. Save document + summary to Persistence (user_id always from JWT)
//  5. Cache result in Redis
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

	// 1. Fast path — already in Redis
	if h.redis != nil {
		if cached, ok, _ := h.redis.Get("summary:" + payload.DocumentID); ok && cached != "" {
			c.Data(http.StatusOK, "application/json", []byte(cached))
			return
		}
	}

	// 2. Already in Persistence?
	persStatus, persBody, _, persErr := h.persistence.Request(
		http.MethodGet,
		"/api/v1/summaries/document/"+payload.DocumentID,
		nil,
		c.Request.Header.Clone(),
	)
	if persErr == nil && persStatus == http.StatusOK {
		if h.redis != nil {
			_ = h.redis.SetEX("summary:"+payload.DocumentID, string(persBody), 3600)
		}
		c.Data(http.StatusOK, "application/json", persBody)
		return
	}

	// 3. Call AI — user_id comes from the verified JWT, never from client body
	userID := c.GetString("user_id")
	if userID == "" {
		problem.Write(c, http.StatusUnauthorized, "Unauthorized",
			"Missing user identity", middleware.CorrelationID(c))
		return
	}

	if !h.ai.Ping("/health") {
		problem.Write(c, http.StatusServiceUnavailable, "Service Unavailable",
			"AI service is not available", middleware.CorrelationID(c))
		return
	}

	aiReqBody, err := json.Marshal(map[string]any{
		"document_id": payload.DocumentID,
		"language":    payload.Language,
	})
	if err != nil {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error",
			"Failed to build AI request", middleware.CorrelationID(c))
		return
	}

	headers := c.Request.Header.Clone()
	headers.Set("Content-Type", "application/json")

	aiStatus, aiBody, _, err := h.ai.Request(http.MethodPost, "/api/v1/summaries/document", aiReqBody, headers)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway",
			"AI service unavailable", middleware.CorrelationID(c))
		return
	}
	if aiStatus != http.StatusOK {
		c.Data(aiStatus, "application/json", aiBody)
		return
	}

	// 4. Parse AI response and save to Persistence
	var aiResp struct {
		DocumentID string `json:"document_id"`
		Filename   string `json:"filename"`
		JobID      string `json:"job_id"`
		Summary    string `json:"summary"`
		ModelUsed  string `json:"model_used"`
	}
	if err := json.Unmarshal(aiBody, &aiResp); err == nil && aiResp.Summary != "" {
		h.saveToPeristence(payload.DocumentID, userID, aiResp.Filename, aiResp.JobID, aiResp.Summary, aiResp.ModelUsed)
	}

	// 5. Cache in Redis and return
	if h.redis != nil {
		_ = h.redis.SetEX("summary:"+payload.DocumentID, string(aiBody), 3600)
	}

	c.Data(http.StatusOK, "application/json", aiBody)
}

// saveToPeristence creates (or reuses) the document record and saves the summary.
// user_id always comes from the Controller's JWT context — never from the AI response or Redis.
func (h *SummariesHandler) saveToPeristence(documentUUID, userID, filename, jobID, summaryContent, modelUsed string) {
	userIDInt, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		log.Printf("[persistence] save skipped: invalid userID=%q err=%v", userID, err)
		return
	}
	if filename == "" {
		filename = fmt.Sprintf("%s.pdf", documentUUID)
	}
	if modelUsed == "" {
		modelUsed = "unknown"
	}

	headers := http.Header{"Content-Type": []string{"application/json"}}

	// Get-or-create document by UUID
	docID := 0
	lookupStatus, lookupBody, _, lookupErr := h.persistence.Request(
		http.MethodGet,
		"/api/v1/documents/uuid/"+documentUUID,
		nil,
		headers,
	)
	if lookupErr != nil {
		log.Printf("[persistence] document lookup error for uuid=%s: %v", documentUUID, lookupErr)
	} else if lookupStatus == http.StatusOK {
		var existing struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(lookupBody, &existing) == nil && existing.ID > 0 {
			docID = existing.ID
			log.Printf("[persistence] reusing document id=%d for uuid=%s", docID, documentUUID)
		}
	}

	if docID == 0 {
		docPayload, _ := json.Marshal(map[string]any{
			"userId":       userIDInt,
			"filename":     filename,
			"filePath":     fmt.Sprintf("/extractions/%s", documentUUID),
			"jobId":        jobID,
			"status":       "COMPLETED",
			"documentUuid": documentUUID,
		})
		createStatus, createBody, _, createErr := h.persistence.Request(
			http.MethodPost, "/api/v1/documents", docPayload, headers,
		)
		if createErr != nil {
			log.Printf("[persistence] document create error: %v", createErr)
			return
		}
		if createStatus >= 300 {
			log.Printf("[persistence] document create returned HTTP %d body=%s", createStatus, string(createBody))
			return
		}
		var created struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(createBody, &created) == nil {
			docID = created.ID
			log.Printf("[persistence] document created id=%d for uuid=%s user=%d", docID, documentUUID, userIDInt)
		}
	}

	if docID == 0 {
		log.Printf("[persistence] could not obtain document id for uuid=%s — summary not saved", documentUUID)
		return
	}

	summaryPayload, _ := json.Marshal(map[string]any{
		"documentId":   docID,
		"content":      summaryContent,
		"modelUsed":    modelUsed,
		"documentUuid": documentUUID,
	})
	summaryStatus, summaryBody, _, summaryErr := h.persistence.Request(
		http.MethodPost, "/api/v1/summaries", summaryPayload, headers,
	)
	if summaryErr != nil {
		log.Printf("[persistence] summary create error: %v", summaryErr)
		return
	}
	if summaryStatus >= 300 {
		log.Printf("[persistence] summary create returned HTTP %d body=%s", summaryStatus, string(summaryBody))
		return
	}
	log.Printf("[persistence] summary saved for document_id=%s user=%d", documentUUID, userIDInt)
}

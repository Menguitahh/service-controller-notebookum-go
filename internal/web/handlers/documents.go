package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	"service-controller-notebookum/internal/config"
	redisclient "service-controller-notebookum/internal/redis"
	"service-controller-notebookum/internal/transport/upstream"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"
	"service-controller-notebookum/internal/web/validators"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DocumentsHandler struct {
	extractor *upstream.Client
	redis     *redisclient.Client
}

func NewDocumentsHandler(cfg config.Config, rc *redisclient.Client) *DocumentsHandler {
	return &DocumentsHandler{
		extractor: upstream.New(cfg.ExtractorURL, 30*time.Second),
		redis:     rc,
	}
}

// Status checks Redis for a completed extraction, then falls back to the extractor service.
// The :id path param is the job_id returned by Upload.
func (h *DocumentsHandler) Status(c *gin.Context) {
	jobID := c.Param("id")

	// Fast path: check Redis for a completed extraction
	if h.redis != nil {
		if docID, ok, _ := h.redis.Get("ctrl:job:" + jobID); ok && docID != "" {
			if done, _ := h.redis.Exists("extraction:" + docID); done {
				c.JSON(http.StatusOK, gin.H{
					"job_id":      jobID,
					"document_id": docID,
					"status":      "completed",
				})
				return
			}
		}
	}

	// Fallback: proxy to extractor
	status, body, _, err := h.extractor.Request(
		http.MethodGet,
		"/internal/v1/extractions/"+jobID,
		nil,
		c.Request.Header.Clone(),
	)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway", "Extractor service unavailable", middleware.CorrelationID(c))
		return
	}
	c.Data(status, "application/json", body)
}

// Upload validates the PDF and forwards it to the extractor service.
// Returns the extractor's job response ({job_id, document_id, status}).
func (h *DocumentsHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		problem.Write(c, http.StatusBadRequest, "Bad Request", "Missing file field", middleware.CorrelationID(c))
		return
	}

	if !validators.ValidatePDFContentType(file.Header.Get("Content-Type")) {
		problem.Write(c, http.StatusBadRequest, "Bad Request", "Only PDF files are accepted (Content-Type must be application/pdf)", middleware.CorrelationID(c))
		return
	}

	src, err := file.Open()
	if err != nil {
		problem.Write(c, http.StatusBadRequest, "Bad Request", "Unable to read file", middleware.CorrelationID(c))
		return
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		problem.Write(c, http.StatusBadRequest, "Bad Request", "Unable to read file", middleware.CorrelationID(c))
		return
	}

	if len(content) > 25*1024*1024 {
		problem.Write(c, http.StatusRequestEntityTooLarge, "Payload Too Large", "File exceeds maximum size of 25MB", middleware.CorrelationID(c))
		return
	}

	documentID := uuid.NewString()
	correlationID := middleware.CorrelationID(c)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Preserve the original Content-Type (application/pdf) — CreateFormFile always
	// sets application/octet-stream which causes the Extractor to reject the upload.
	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition", fmt.Sprintf(
		`form-data; name="file"; filename="%s"`,
		strings.NewReplacer(`"`, `%22`, `\`, `%5C`).Replace(filepath.Base(file.Filename)),
	))
	fileHeader.Set("Content-Type", "application/pdf")

	fw, err := mw.CreatePart(fileHeader)
	if err != nil {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error", "Failed to build upload", middleware.CorrelationID(c))
		return
	}
	if _, err = fw.Write(content); err != nil {
		problem.Write(c, http.StatusInternalServerError, "Internal Server Error", "Failed to build upload", middleware.CorrelationID(c))
		return
	}
	_ = mw.WriteField("document_id", documentID)
	_ = mw.WriteField("correlation_id", correlationID)
	_ = mw.WriteField("user_id", c.GetString("user_id"))
	mw.Close()

	status, body, _, err := h.extractor.RequestMultipart(
		"/internal/v1/extractions",
		mw.FormDataContentType(),
		&buf,
		c.Request.Header.Clone(),
	)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway", "Extractor service unavailable", middleware.CorrelationID(c))
		return
	}

	// Cache job_id → document_id so Status can use Redis for the fast path
	if h.redis != nil {
		var result struct {
			JobID string `json:"job_id"`
		}
		if json.Unmarshal(body, &result) == nil && result.JobID != "" {
			_ = h.redis.SetEX("ctrl:job:"+result.JobID, documentID, 4000)
		}
	}

	c.Data(status, "application/json", body)
}

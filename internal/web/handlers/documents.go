package handlers

import (
	"io"
	"net/http"
	"path/filepath"

	"service-controller-notebookum/internal/domain/documents"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"
	"service-controller-notebookum/internal/web/validators"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DocumentsHandler struct {
	store *documents.Store
}

func NewDocumentsHandler(store *documents.Store) *DocumentsHandler {
	return &DocumentsHandler{store: store}
}

func (h *DocumentsHandler) Status(c *gin.Context) {
	rec, err := h.store.RequireOwner(c.Param("id"), c.GetString("user_id"))
	if err != nil {
		if err.Error() == "forbidden" {
			problem.Write(c, http.StatusForbidden, "Forbidden", "Access denied", middleware.CorrelationID(c))
			return
		}
		problem.Write(c, http.StatusNotFound, "Not Found", "Document not found", middleware.CorrelationID(c))
		return
	}

	code := http.StatusAccepted
	if rec.Status == "ready" {
		code = http.StatusOK
	}
	c.JSON(code, gin.H{"document_id": rec.ID, "status": rec.Status})
}

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

	rec := h.store.Upsert(c.GetString("user_id"), filepath.Base(file.Filename), content)
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	c.JSON(http.StatusAccepted, gin.H{"document_id": rec.ID, "status": "accepted"})
}

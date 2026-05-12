package handlers

import (
	"net/http"

	"service-controller-notebookum/internal/domain/documents"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

type SummariesHandler struct {
	store *documents.Store
}

func NewSummariesHandler(store *documents.Store) *SummariesHandler {
	return &SummariesHandler{store: store}
}

func (h *SummariesHandler) Get(c *gin.Context) {
	rec, err := h.store.RequireOwner(c.Param("id"), c.GetString("user_id"))
	if err != nil {
		if err.Error() == "forbidden" {
			problem.Write(c, http.StatusForbidden, "Forbidden", "Access denied", middleware.CorrelationID(c))
			return
		}
		problem.Write(c, http.StatusNotFound, "Not Found", "Document not found", middleware.CorrelationID(c))
		return
	}

	if rec.Summary == "" {
		problem.Write(c, http.StatusAccepted, "Accepted", "Document still processing", middleware.CorrelationID(c))
		return
	}

	c.JSON(http.StatusOK, gin.H{"document_id": rec.ID, "summary": rec.Summary, "status": "ready"})
}

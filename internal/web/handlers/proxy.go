package handlers

import (
	"service-controller-notebookum/internal/services/strangler"

	"github.com/gin-gonic/gin"
)

type ProxyHandler struct {
	router *strangler.Router
}

func NewProxyHandler(router *strangler.Router) *ProxyHandler {
	return &ProxyHandler{router: router}
}

func (h *ProxyHandler) Handle(c *gin.Context) {
	h.router.Handle(c)
}

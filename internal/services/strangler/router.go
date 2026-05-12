package strangler

import (
	"io"
	"net/http"
	"strings"
	"time"

	"service-controller-notebookum/internal/config"
	"service-controller-notebookum/internal/transport/upstream"
	"service-controller-notebookum/internal/web/middleware"
	"service-controller-notebookum/internal/web/problem"

	"github.com/gin-gonic/gin"
)

type Router struct {
	cfg     config.Config
	clients map[string]*upstream.Client
}

func NewRouter(cfg config.Config) *Router {
	return &Router{
		cfg: cfg,
		clients: map[string]*upstream.Client{
			"monolith":            upstream.New(cfg.MonolithURL, 10*time.Second),
			"service-user":        upstream.New(cfg.UserServiceURL, 5*time.Second),
			"service-persistence": upstream.New(cfg.PersistenceURL, 5*time.Second),
			"service-extractor":   upstream.New(cfg.ExtractorURL, 10*time.Second),
			"ai":                  upstream.New(cfg.AIURL, 5*time.Second),
		},
	}
}

func (r *Router) Handle(c *gin.Context) {
	fullPath := "/api/v1" + c.Param("path")
	if fullPath == "/api/v1" {
		fullPath = "/api/v1/"
	}

	destination := "monolith"
	if r.cfg.StranglerEnableMSRouting {
		if rule, err := FindRule(c.Request.Method, fullPath, ""); err == nil && rule != nil {
			destination = rule.Destination
		}
	}
#revisar strangler 
	client := r.clients[destination]
	if client == nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway", "Service unavailable", middleware.CorrelationID(c))
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		problem.Write(c, http.StatusBadGateway, "Bad Gateway", "Service unavailable", middleware.CorrelationID(c))
		return
	}

	headers := c.Request.Header.Clone()
	headers.Del("Host")

	status, respBody, respHeaders, err := client.Request(c.Request.Method, fullPath, body, headers)
	if err != nil {
		problem.Write(c, http.StatusServiceUnavailable, "Service Unavailable", "Upstream unavailable", middleware.CorrelationID(c))
		return
	}

	for k, values := range respHeaders {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		for _, v := range values {
			c.Writer.Header().Add(k, v)
		}
	}

	contentType := respHeaders.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(status, contentType, respBody)
}

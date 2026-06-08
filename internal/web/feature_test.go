package web

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"service-controller-notebookum/internal/config"

	"github.com/gin-gonic/gin"
)

func featureRouter(t *testing.T, userURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return NewRouter(config.Config{
		CorrelationHeader: "X-Correlation-ID",
		UserServiceURL:    userURL,
	})
}

func featureRouterFull(t *testing.T, userURL, extractorURL, aiURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	return NewRouter(config.Config{
		CorrelationHeader: "X-Correlation-ID",
		UserServiceURL:    userURL,
		ExtractorURL:      extractorURL,
		AIURL:             aiURL,
	})
}

func TestCreateUserRoutesToUpstream(t *testing.T) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-User-ID"); got != "creator" {
			t.Fatalf("expected user header, got %q", got)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"u1","name":"John"}`))
	}))
	defer upstream.Close()

	router := featureRouter(t, upstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{"name":"John","email":"john@example.com","password":"pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "creator")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "u1" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestDocumentUploadAndStatus(t *testing.T) {
	// Mock extractor: handles upload and status
	extractorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":"job-1","document_id":"doc-1","status":"processing"}`))
			return
		}
		// GET status
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id":"job-1","document_id":"doc-1","status":"processing"}`))
	}))
	defer extractorServer.Close()

	// Mock AI: handles summary creation
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"processing"}`))
	}))
	defer aiServer.Close()

	router := featureRouterFull(t, "", extractorServer.URL, aiServer.URL)

	// --- Upload ---
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreatePart(textprotoMIMEHeader("file", "doc.pdf", "application/pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("%PDF-1.4 test")); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/documento/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "u1")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("upload: expected 202, got %d — body: %s", resp.Code, resp.Body.String())
	}

	var uploaded struct {
		JobID string `json:"job_id"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &uploaded); err != nil {
		t.Fatal(err)
	}

	// --- Status (uses job_id) ---
	jobID := uploaded.JobID
	if jobID == "" {
		jobID = "job-1"
	}
	statusReq := httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+jobID+"/status", nil)
	statusReq.Header.Set("X-User-ID", "u1")
	statusResp := httptest.NewRecorder()
	router.ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusAccepted {
		t.Fatalf("status: expected 202, got %d — body: %s", statusResp.Code, statusResp.Body.String())
	}

	// --- Summary (POST with document_id) ---
	summaryBody := bytes.NewBufferString(`{"document_id":"doc-1","language":"es"}`)
	summaryReq := httptest.NewRequest(http.MethodPost, "/api/v1/summaries/document", summaryBody)
	summaryReq.Header.Set("Content-Type", "application/json")
	summaryReq.Header.Set("X-User-ID", "u1")
	summaryResp := httptest.NewRecorder()
	router.ServeHTTP(summaryResp, summaryReq)
	if summaryResp.Code != http.StatusAccepted {
		t.Fatalf("summary: expected 202, got %d — body: %s", summaryResp.Code, summaryResp.Body.String())
	}
}

func textprotoMIMEHeader(field, filename, contentType string) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	return h
}

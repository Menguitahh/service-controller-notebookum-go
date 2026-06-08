package upstream

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrUnavailable = errors.New("upstream unavailable")
	ErrBadGateway  = errors.New("bad gateway")
)

type Client struct {
	BaseURL string
	Timeout time.Duration
	Client  *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Timeout: timeout,
		Client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec — internal network with self-signed certs
			},
		},
	}
}

func (c *Client) Request(method, path string, body []byte, headers http.Header) (int, []byte, http.Header, error) {
	target, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}
	if headers != nil {
		req.Header = headers.Clone()
		req.Header.Del("Content-Length")
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, nil, nil, ErrUnavailable
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}

	return resp.StatusCode, payload, resp.Header.Clone(), nil
}

// Ping does a lightweight health check to path using a 2-second timeout.
// Returns true when the service responds with HTTP status < 500.
func (c *Client) Ping(path string) bool {
	target, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return false
	}
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return false
	}
	probe := &http.Client{Timeout: 2 * time.Second, Transport: c.Client.Transport}
	resp, err := probe.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

// RequestMultipart forwards a pre-built multipart body to the upstream service.
// Used to proxy file uploads without buffering the entire stream twice.
func (c *Client) RequestMultipart(path, contentType string, body *bytes.Buffer, passthroughHeaders http.Header) (int, []byte, http.Header, error) {
	target, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}

	req, err := http.NewRequest(http.MethodPost, target, body)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}
	req.Header.Set("Content-Type", contentType)

	// Forward select headers (correlation ID, auth) but not content-related ones
	for key, vals := range passthroughHeaders {
		k := strings.ToLower(key)
		if k == "content-type" || k == "content-length" || k == "transfer-encoding" {
			continue
		}
		for _, v := range vals {
			req.Header.Add(key, v)
		}
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return 0, nil, nil, ErrUnavailable
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, nil, ErrBadGateway
	}

	return resp.StatusCode, payload, resp.Header.Clone(), nil
}

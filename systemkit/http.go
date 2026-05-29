package systemkit

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fluxplane/fluxplane-system"
)

const (
	defaultHTTPTimeout  = 30 * time.Second
	defaultHTTPMaxBytes = 512 * 1024
)

// HTTPRequest is a neutral HTTP request shape.
type HTTPRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	Body      []byte
	Timeout   time.Duration
	MaxBytes  int
	UserAgent string
	TLSConfig *tls.Config
}

// HTTPResponse is a neutral HTTP response shape.
type HTTPResponse struct {
	URL         string              `json:"url"`
	FinalURL    string              `json:"final_url,omitempty"`
	Method      string              `json:"method,omitempty"`
	Status      string              `json:"status,omitempty"`
	StatusCode  int                 `json:"status_code,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	ContentType string              `json:"content_type,omitempty"`
	Body        []byte              `json:"body,omitempty"`
	Truncated   bool                `json:"truncated,omitempty"`
	Duration    time.Duration       `json:"-"`
}

type HTTPOption func(*httpConfig)

type httpConfig struct {
	timeout   time.Duration
	maxBytes  int
	tlsConfig *tls.Config
}

// WithHTTPClientTimeout sets the fallback request timeout when a request
// context does not already have a deadline.
func WithHTTPClientTimeout(timeout time.Duration) HTTPOption {
	return func(out *httpConfig) {
		if timeout > 0 {
			out.timeout = timeout
		}
	}
}

// WithHTTPClientMaxBytes sets the maximum response body size requested through
// DoHTTP-backed clients.
func WithHTTPClientMaxBytes(maxBytes int) HTTPOption {
	return func(out *httpConfig) {
		if maxBytes > 0 {
			out.maxBytes = maxBytes
		}
	}
}

// WithHTTPClientTLSConfig sets the TLS configuration forwarded to HTTP clients.
func WithHTTPClientTLSConfig(cfg *tls.Config) HTTPOption {
	return WithTLSConfig(cfg)
}

func WithTLSConfig(cfg *tls.Config) HTTPOption {
	return func(out *httpConfig) {
		if cfg != nil {
			out.tlsConfig = secureTLSConfig(cfg)
		}
	}
}

// HTTPClient returns an http.Client that dials through the wrapped system network.
func (f Facade) HTTPClient(opts ...HTTPOption) *http.Client {
	cfg := httpConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return NewHTTPClient(f.Network(), opts...)
}

// NewHTTPClient returns an http.Client that routes requests through network.
func NewHTTPClient(network system.Network, opts ...HTTPOption) *http.Client {
	return &http.Client{Transport: NewRoundTripper(network, opts...)}
}

// NewRoundTripper returns an http.RoundTripper that routes requests through
// DoHTTP so request timeouts, body limits, and TLS options stay consistent.
func NewRoundTripper(network system.Network, opts ...HTTPOption) http.RoundTripper {
	cfg := httpConfig{timeout: defaultHTTPTimeout, maxBytes: defaultHTTPMaxBytes}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return roundTripper{network: network, cfg: cfg}
}

type roundTripper struct {
	network system.Network
	cfg     httpConfig
}

func (t roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.network == nil {
		return nil, fmt.Errorf("systemkit: network is nil")
	}
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
	}
	headers := map[string]string{}
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	timeout := t.cfg.timeout
	if deadline, ok := req.Context().Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			timeout = remaining
		}
	}
	resp, err := DoHTTP(req.Context(), t.network, HTTPRequest{
		URL:       req.URL.String(),
		Method:    req.Method,
		Headers:   headers,
		Body:      body,
		Timeout:   timeout,
		MaxBytes:  t.cfg.maxBytes,
		UserAgent: req.UserAgent(),
		TLSConfig: t.cfg.tlsConfig,
	})
	if err != nil {
		return nil, err
	}
	httpResp := &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Header:        http.Header(resp.Headers),
		Body:          io.NopCloser(bytes.NewReader(resp.Body)),
		ContentLength: int64(len(resp.Body)),
		Request:       req,
	}
	if httpResp.Status == "" {
		httpResp.Status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return httpResp, nil
}

// NewHTTPTransport returns an http.RoundTripper that dials through network.
func NewHTTPTransport(network system.Network, tlsConfig *tls.Config) http.RoundTripper {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           network.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSClientConfig:       secureTLSConfig(tlsConfig),
		TLSHandshakeTimeout:   30 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

// DoHTTP executes req through the wrapped system network.
func (f Facade) DoHTTP(ctx context.Context, req HTTPRequest) (HTTPResponse, error) {
	return DoHTTP(ctx, f.Network(), req)
}

// DoHTTP executes req through network.
func DoHTTP(ctx context.Context, network system.Network, req HTTPRequest) (HTTPResponse, error) {
	if network == nil {
		return HTTPResponse{}, fmt.Errorf("systemkit: network is nil")
	}
	if doer, ok := network.(interface {
		DoHTTP(context.Context, HTTPRequest) (HTTPResponse, error)
	}); ok {
		return doer.DoHTTP(ctx, req)
	}
	parsed, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil {
		return HTTPResponse{}, err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return HTTPResponse{}, fmt.Errorf("url must be absolute http or https")
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !AllowedHTTPMethod(method) {
		return HTTPResponse{}, fmt.Errorf("unsupported HTTP method %q", method)
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, method, parsed.String(), bytes.NewReader(req.Body))
	if err != nil {
		return HTTPResponse{}, err
	}
	if req.UserAgent != "" {
		httpReq.Header.Set("User-Agent", req.UserAgent)
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	client := &http.Client{Transport: NewHTTPTransport(network, req.TLSConfig)}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultHTTPMaxBytes
	}
	start := time.Now()
	resp, err := client.Do(httpReq)
	if err != nil {
		return HTTPResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	if err != nil {
		return HTTPResponse{}, err
	}
	truncated := len(body) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	return HTTPResponse{
		URL: parsed.String(), FinalURL: resp.Request.URL.String(), Method: method,
		Status: resp.Status, StatusCode: resp.StatusCode, Headers: resp.Header,
		ContentType: resp.Header.Get("Content-Type"), Body: body, Truncated: truncated,
		Duration: time.Since(start),
	}, nil
}

// AllowedHTTPMethod reports whether method is enabled for DoHTTP.
func AllowedHTTPMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

func secureTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return &tls.Config{MinVersion: tls.VersionTLS12}
	}
	out := cfg.Clone()
	if out.MinVersion == 0 || out.MinVersion < tls.VersionTLS12 {
		out.MinVersion = tls.VersionTLS12
	}
	return out
}

package msa

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout     = 30 * time.Second
	defaultSessionTTL  = 25 * time.Minute
	maxBodySize        = 4 << 20
	defaultMaxAttempts = 3
)

type Config struct {
	Endpoint    string
	Username    string
	Password    string
	InsecureTLS bool
	Timeout     time.Duration
	SessionTTL  time.Duration
	Retry       RetryConfig
}

type Client struct {
	baseURL     string
	username    string
	password    string
	httpClient  *http.Client
	retryConfig RetryConfig
	sessionTTL  time.Duration

	mu           sync.Mutex
	sessionKey   string
	sessionUntil time.Time
}

func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("endpoint is required")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return nil, errors.New("username is required")
	}
	if strings.TrimSpace(cfg.Password) == "" {
		return nil, errors.New("password is required")
	}

	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("endpoint must include scheme and host")
	}

	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	retryConfig := cfg.Retry.withDefaults(defaultMaxAttempts)
	sessionTTL := cfg.SessionTTL
	if sessionTTL == 0 {
		sessionTTL = defaultSessionTTL
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.InsecureTLS}

	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}

	return &Client{
		baseURL:     endpoint,
		username:    cfg.Username,
		password:    cfg.Password,
		httpClient:  client,
		retryConfig: retryConfig,
		sessionTTL:  sessionTTL,
	}, nil
}

func (c *Client) Login(ctx context.Context) (string, error) {
	hash := loginHash(c.username, c.password)
	loginURL := fmt.Sprintf("%s/api/login/%s", c.baseURL, hash)

	body, _, status, err := c.getWithRetry(ctx, loginURL, nil)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("login unexpected HTTP status %d", status)
	}

	response, err := parseResponse(body)
	if err != nil {
		return "", fmt.Errorf("login response parse failed: %w", err)
	}

	statusObj, ok := response.Status()
	if !ok {
		return "", errors.New("login response missing status object")
	}
	if !statusObj.Success() {
		return "", fmt.Errorf("login failed: %s", statusObj.Response)
	}
	if statusObj.Response == "" {
		return "", errors.New("login response missing session key")
	}

	return statusObj.Response, nil
}

func (c *Client) Logout(ctx context.Context, sessionKey string) error {
	if strings.TrimSpace(sessionKey) == "" {
		return errors.New("session key is required")
	}

	logoutURL := fmt.Sprintf("%s/api/exit", c.baseURL)
	headers := map[string]string{"sessionKey": sessionKey}
	body, _, status, err := c.getWithRetry(ctx, logoutURL, headers)
	if err != nil {
		return fmt.Errorf("logout request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("logout unexpected HTTP status %d", status)
	}

	response, err := parseResponse(body)
	if err != nil {
		return fmt.Errorf("logout response parse failed: %w", err)
	}

	statusObj, ok := response.Status()
	if !ok {
		return errors.New("logout response missing status object")
	}
	if !statusObj.Success() {
		return fmt.Errorf("logout failed: %s", statusObj.Response)
	}

	return nil
}

func (c *Client) Do(ctx context.Context, sessionKey, path string, query url.Values) (Response, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return Response{}, errors.New("session key is required")
	}
	if path == "" {
		return Response{}, errors.New("path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	fullURL := c.baseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	headers := map[string]string{"sessionKey": sessionKey}
	body, _, status, err := c.getWithRetry(ctx, fullURL, headers)
	if err != nil {
		return Response{}, fmt.Errorf("request failed: %w", err)
	}
	if status != http.StatusOK {
		return Response{}, fmt.Errorf("unexpected HTTP status %d", status)
	}

	response, err := parseResponse(body)
	if err != nil {
		return Response{}, fmt.Errorf("response parse failed: %w", err)
	}

	if statusObj, ok := response.Status(); ok && !statusObj.Success() {
		return Response{}, APIError{Status: statusObj}
	}

	return response, nil
}

func (c *Client) Command(ctx context.Context, sessionKey string, parts ...string) (Response, error) {
	return c.Do(ctx, sessionKey, CommandPath(parts...), nil)
}

func (c *Client) Execute(ctx context.Context, parts ...string) (Response, error) {
	sessionKey, err := c.ensureSession(ctx)
	if err != nil {
		return Response{}, err
	}

	resp, err := c.Command(ctx, sessionKey, parts...)
	if err == nil {
		return resp, nil
	}

	if IsSessionError(err) {
		c.invalidateSession()
		sessionKey, err = c.ensureSession(ctx)
		if err != nil {
			return Response{}, err
		}
		return c.Command(ctx, sessionKey, parts...)
	}

	return Response{}, err
}

func loginHash(username, password string) string {
	sum := sha256.Sum256([]byte(username + "_" + password))
	return hex.EncodeToString(sum[:])
}

func parseResponse(body []byte) (Response, error) {
	var response Response
	if err := xml.Unmarshal(body, &response); err != nil {
		return Response{}, err
	}
	return response, nil
}

func (c *Client) ensureSession(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionKey != "" && time.Now().Before(c.sessionUntil) {
		return c.sessionKey, nil
	}

	sessionKey, err := c.Login(ctx)
	if err != nil {
		return "", err
	}

	c.sessionKey = sessionKey
	c.sessionUntil = time.Now().Add(c.sessionTTL)

	return sessionKey, nil
}

func (c *Client) invalidateSession() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.sessionKey = ""
	c.sessionUntil = time.Time{}
}

func (c *Client) getWithRetry(ctx context.Context, url string, headers map[string]string) ([]byte, http.Header, int, error) {
	var lastBody []byte
	var lastHeader http.Header
	var lastStatus int

	err := doWithRetry(ctx, c.retryConfig, func() (bool, error) {
		body, header, status, err := c.get(ctx, url, headers)
		lastBody = body
		lastHeader = header
		lastStatus = status
		if err != nil {
			return true, err
		}
		if isRetryableStatus(status) {
			return true, fmt.Errorf("retryable HTTP status %d", status)
		}
		return false, nil
	})
	if err != nil {
		return lastBody, lastHeader, lastStatus, err
	}
	return lastBody, lastHeader, lastStatus, nil
}

func (c *Client) get(ctx context.Context, url string, headers map[string]string) ([]byte, http.Header, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, 0, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, nil, resp.StatusCode, err
	}

	return body, resp.Header, resp.StatusCode, nil
}

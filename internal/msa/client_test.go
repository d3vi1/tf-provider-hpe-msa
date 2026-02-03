package msa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginSuccess(t *testing.T) {
	fixture := readFixture(t, "login_success.xml")
	expectedHash := loginHash("user", "pass", "_!")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login/"+expectedHash {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	key, err := client.Login(context.Background())
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if key != "session-key-123" {
		t.Fatalf("unexpected session key: %q", key)
	}
}

func TestLoginFailure(t *testing.T) {
	fixture := readFixture(t, "login_error.xml")
	expectedHash1 := loginHash("user", "pass", "_!")
	expectedHash2 := loginHash("user", "pass", "_")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/login/"+expectedHash1 && r.URL.Path != "/api/login/"+expectedHash2 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Login(context.Background())
	if err == nil {
		t.Fatalf("expected login error")
	}
}

func TestDoSendsSessionKey(t *testing.T) {
	fixture := readFixture(t, "command_success.xml")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show/system" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("sessionKey") != "abc123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Do(context.Background(), "abc123", "/api/show/system", url.Values{})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestDoRetriesOn503(t *testing.T) {
	fixture := readFixture(t, "command_success.xml")
	callCount := 0

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/show/system" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{
		MaxAttempts: 2,
		MinBackoff:  time.Millisecond,
		MaxBackoff:  time.Millisecond,
		Jitter:      0,
	}

	_, err := client.Do(context.Background(), "abc123", "/api/show/system", url.Values{})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", callCount)
	}
}

func TestExecuteRetriesOnSessionError(t *testing.T) {
	commandOK := readFixture(t, "command_success.xml")
	commandError := readFixture(t, "session_error.xml")

	loginCalls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/login/"):
			loginCalls++
			w.Header().Set("Content-Type", "text/xml")
			if loginCalls == 1 {
				_, _ = w.Write(loginResponse("session-1"))
				return
			}
			_, _ = w.Write(loginResponse("session-2"))
		case r.URL.Path == "/api/show/system":
			w.Header().Set("Content-Type", "text/xml")
			if r.Header.Get("sessionKey") == "session-1" {
				_, _ = w.Write(commandError)
				return
			}
			_, _ = w.Write(commandOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{
		MaxAttempts: 1,
	}
	client.sessionTTL = time.Minute

	_, err := client.Execute(context.Background(), "show", "system")
	if err != nil {
		t.Fatalf("expected session retry success, got %v", err)
	}
	if loginCalls < 2 {
		t.Fatalf("expected login retry, got %d logins", loginCalls)
	}
}

func TestFindActiveVolumeCopyJobWithETA(t *testing.T) {
	fixture := readFixture(t, "show_volume_copy_active_eta.xml")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/login/"):
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(loginResponse("session-eta"))
		case r.URL.Path == "/api/show/volume-copy":
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(fixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{MaxAttempts: 1}

	job, err := client.FindActiveVolumeCopyJob(context.Background(), "snap-prod-001", "clone-prod-001")
	if err != nil {
		t.Fatalf("unexpected lookup error: %v", err)
	}
	if job == nil {
		t.Fatalf("expected active volume-copy job")
	}
	if job.ID != "job-77" {
		t.Fatalf("expected job-77, got %q", job.ID)
	}
	if !job.HasETA {
		t.Fatalf("expected ETA to be available")
	}
	if job.ETA != 2*time.Minute {
		t.Fatalf("expected 2m ETA, got %s", job.ETA)
	}
}

func TestFindActiveVolumeCopyJobWithoutETA(t *testing.T) {
	fixture := readFixture(t, "show_volume_copy_active_no_eta.xml")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/login/"):
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(loginResponse("session-no-eta"))
		case r.URL.Path == "/api/show/volume-copy":
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(fixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{MaxAttempts: 1}

	job, err := client.FindActiveVolumeCopyJob(context.Background(), "snap-prod-001", "clone-prod-001")
	if err != nil {
		t.Fatalf("unexpected lookup error: %v", err)
	}
	if job == nil {
		t.Fatalf("expected active volume-copy job")
	}
	if job.ID != "job-90" {
		t.Fatalf("expected job-90, got %q", job.ID)
	}
	if job.HasETA {
		t.Fatalf("did not expect ETA to be available")
	}
	if job.ETARaw != "N/A" {
		t.Fatalf("expected raw ETA marker N/A, got %q", job.ETARaw)
	}
}

func TestFindActiveVolumeCopyJobFallsBackToVolumeCopiesCommand(t *testing.T) {
	fixture := readFixture(t, "show_volume_copy_active_eta.xml")
	volumeCopyCalls := 0
	volumeCopiesCalls := 0

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/login/"):
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(loginResponse("session-fallback"))
		case r.URL.Path == "/api/show/volume-copy":
			volumeCopyCalls++
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(commandErrorResponse("Unsupported command"))
		case r.URL.Path == "/api/show/volume-copies":
			volumeCopiesCalls++
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(fixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{MaxAttempts: 1}

	job, err := client.FindActiveVolumeCopyJob(context.Background(), "snap-prod-001", "clone-prod-001")
	if err != nil {
		t.Fatalf("unexpected lookup error: %v", err)
	}
	if job == nil {
		t.Fatalf("expected active volume-copy job")
	}
	if volumeCopyCalls != 1 {
		t.Fatalf("expected one volume-copy call, got %d", volumeCopyCalls)
	}
	if volumeCopiesCalls != 1 {
		t.Fatalf("expected one volume-copies call, got %d", volumeCopiesCalls)
	}
}

func TestFindActiveVolumeCopyJobFallsBackWhenPrimaryHasNoActiveJobs(t *testing.T) {
	emptyFixture := readFixture(t, "command_success.xml")
	fallbackFixture := readFixture(t, "show_volume_copy_active_eta.xml")
	volumeCopyCalls := 0
	volumeCopiesCalls := 0

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/login/"):
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(loginResponse("session-fallback-empty"))
		case r.URL.Path == "/api/show/volume-copy":
			volumeCopyCalls++
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(emptyFixture)
		case r.URL.Path == "/api/show/volume-copies":
			volumeCopiesCalls++
			w.Header().Set("Content-Type", "text/xml")
			_, _ = w.Write(fallbackFixture)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.retryConfig = RetryConfig{MaxAttempts: 1}

	job, err := client.FindActiveVolumeCopyJob(context.Background(), "snap-prod-001", "clone-prod-001")
	if err != nil {
		t.Fatalf("unexpected lookup error: %v", err)
	}
	if job == nil {
		t.Fatalf("expected active volume-copy job from fallback command")
	}
	if job.ID != "job-77" {
		t.Fatalf("expected fallback job job-77, got %q", job.ID)
	}
	if volumeCopyCalls != 1 {
		t.Fatalf("expected one volume-copy call, got %d", volumeCopyCalls)
	}
	if volumeCopiesCalls != 1 {
		t.Fatalf("expected one volume-copies call, got %d", volumeCopiesCalls)
	}
}

func TestParseVolumeCopyETA(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		expected  time.Duration
		expectETA bool
	}{
		{name: "hhmmss", value: "00:01:30", expected: 90 * time.Second, expectETA: true},
		{name: "seconds", value: "120", expected: 120 * time.Second, expectETA: true},
		{name: "duration", value: "2m 30s", expected: 150 * time.Second, expectETA: true},
		{name: "human", value: "3 minutes 5 seconds", expected: 185 * time.Second, expectETA: true},
		{name: "missing", value: "N/A", expected: 0, expectETA: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, ok := parseVolumeCopyETA(tc.value)
			if ok != tc.expectETA {
				t.Fatalf("expected ETA available %t, got %t", tc.expectETA, ok)
			}
			if result != tc.expected {
				t.Fatalf("expected %s, got %s", tc.expected, result)
			}
		})
	}
}

func TestCommandPath(t *testing.T) {
	path := CommandPath("show", "pools")
	if path != "/api/show/pools" {
		t.Fatalf("unexpected command path: %s", path)
	}
}

func newTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		Endpoint:    endpoint,
		Username:    "user",
		Password:    "pass",
		InsecureTLS: true,
	})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	return client
}

func loginResponse(sessionKey string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<RESPONSE VERSION="L100">
  <OBJECT basetype="status" name="status" oid="1">
    <PROPERTY name="response-type" type="string">Success</PROPERTY>
    <PROPERTY name="response-type-numeric" type="uint32">0</PROPERTY>
    <PROPERTY name="response" type="string">` + sessionKey + `</PROPERTY>
    <PROPERTY name="return-code" type="sint32">1</PROPERTY>
  </OBJECT>
</RESPONSE>`)
}

func commandErrorResponse(message string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<RESPONSE VERSION="L100">
  <OBJECT basetype="status" name="status" oid="1">
    <PROPERTY name="response-type" type="string">Error</PROPERTY>
    <PROPERTY name="response-type-numeric" type="uint32">1</PROPERTY>
    <PROPERTY name="response" type="string">` + message + `</PROPERTY>
    <PROPERTY name="return-code" type="sint32">-1</PROPERTY>
  </OBJECT>
</RESPONSE>`)
}

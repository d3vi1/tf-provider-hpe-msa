package msa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginSuccess(t *testing.T) {
	fixture := readFixture(t, "login_success.xml")
	expectedHash := loginHash("user", "pass")

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
	expectedHash := loginHash("user", "pass")

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

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
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

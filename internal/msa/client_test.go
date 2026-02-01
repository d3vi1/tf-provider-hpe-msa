package msa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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

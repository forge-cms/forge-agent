package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestConfigDefaults verifies that setDefaults fills in zero values.
func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	cfg.setDefaults()

	if cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-sonnet-4-6")
	}
	if cfg.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10", cfg.MaxTurns)
	}
}

func TestConfigDefaultsPreservesExistingValues(t *testing.T) {
	cfg := Config{Model: "claude-opus-4-7", MaxTurns: 5}
	cfg.setDefaults()

	if cfg.Model != "claude-opus-4-7" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-opus-4-7")
	}
	if cfg.MaxTurns != 5 {
		t.Errorf("MaxTurns = %d, want 5", cfg.MaxTurns)
	}
}

// TestHttpGet exercises the built-in http_get handler against a local server.
func TestHttpGet(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "hello world")
		}))
		defer srv.Close()

		result, err := httpGet(context.Background(), map[string]any{"url": srv.URL})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "hello world" {
			t.Errorf("result = %q, want %q", result, "hello world")
		}
	})

	t.Run("non-2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}))
		defer srv.Close()

		result, err := httpGet(context.Background(), map[string]any{"url": srv.URL})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(result, "HTTP 404:") {
			t.Errorf("result = %q, want HTTP 404: prefix", result)
		}
	})

	t.Run("missing url", func(t *testing.T) {
		_, err := httpGet(context.Background(), map[string]any{})
		if err == nil {
			t.Fatal("expected error for missing url")
		}
	})
}

// TestHttpPost exercises the built-in http_post handler against a local server.
func TestHttpPost(t *testing.T) {
	t.Run("text/plain", func(t *testing.T) {
		var gotBody, gotCT string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCT = r.Header.Get("Content-Type")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			fmt.Fprint(w, "ok")
		}))
		defer srv.Close()

		result, err := httpPost(context.Background(), map[string]any{
			"url":  srv.URL,
			"body": "ping",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(result, "HTTP 200:") {
			t.Errorf("result = %q, want HTTP 200: prefix", result)
		}
		if gotBody != "ping" {
			t.Errorf("body = %q, want %q", gotBody, "ping")
		}
		if gotCT != "text/plain" {
			t.Errorf("Content-Type = %q, want %q", gotCT, "text/plain")
		}
	})

	t.Run("application/json", func(t *testing.T) {
		var gotCT string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCT = r.Header.Get("Content-Type")
			fmt.Fprint(w, `{"ok":true}`)
		}))
		defer srv.Close()

		_, err := httpPost(context.Background(), map[string]any{
			"url":          srv.URL,
			"body":         `{"msg":"hello"}`,
			"content_type": "application/json",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotCT != "application/json" {
			t.Errorf("Content-Type = %q, want %q", gotCT, "application/json")
		}
	})

	t.Run("missing url", func(t *testing.T) {
		_, err := httpPost(context.Background(), map[string]any{"body": "x"})
		if err == nil {
			t.Fatal("expected error for missing url")
		}
	})
}

// TestClip verifies the body truncation helper.
func TestClip(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}
	for _, tc := range tests {
		got := clip(tc.input, tc.n)
		if got != tc.want {
			t.Errorf("clip(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
		}
	}
}

// TestBuiltinToolNames verifies that the two built-in tools have the expected names.
func TestBuiltinToolNames(t *testing.T) {
	bt := builtinTools()
	if len(bt) != 2 {
		t.Fatalf("len(builtinTools()) = %d, want 2", len(bt))
	}
	names := []string{bt[0].param.OfTool.Name, bt[1].param.OfTool.Name}
	if names[0] != "http_get" {
		t.Errorf("bt[0].Name = %q, want %q", names[0], "http_get")
	}
	if names[1] != "http_post" {
		t.Errorf("bt[1].Name = %q, want %q", names[1], "http_post")
	}
}

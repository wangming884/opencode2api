package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameListenPort(t *testing.T) {
	tests := []struct {
		name   string
		first  string
		second string
		want   bool
	}{
		{name: "exact", first: "0.0.0.0:8080", second: "0.0.0.0:8080", want: true},
		{name: "same port different host", first: "0.0.0.0:8080", second: "127.0.0.1:8080", want: true},
		{name: "different port", first: "0.0.0.0:8080", second: "127.0.0.1:8081", want: false},
		{name: "invalid addresses", first: "localhost", second: "localhost:8080", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameListenPort(tt.first, tt.second); got != tt.want {
				t.Fatalf("sameListenPort(%q, %q) = %v, want %v", tt.first, tt.second, got, tt.want)
			}
		})
	}
}

func TestCombinedHandlerRoutesAPIAndWebUI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api"))
	})
	webui := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("webui"))
	})
	handler := combinedHandler(api, webui)

	tests := []struct {
		path string
		want string
	}{
		{path: "/v1", want: "api"},
		{path: "/v1/models", want: "api"},
		{path: "/v1/chat/completions", want: "api"},
		{path: "/healthz", want: "api"},
		{path: "/", want: "webui"},
		{path: "/api/config", want: "webui"},
		{path: "/settings", want: "webui"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if got := recorder.Body.String(); got != tt.want {
				t.Fatalf("path %q routed to %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

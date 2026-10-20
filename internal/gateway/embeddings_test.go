package gateway

import "testing"

func TestExternalEmbeddingsEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":            "https://api.openai.com/v1/embeddings",
		"https://api.openai.com/v1/":           "https://api.openai.com/v1/embeddings",
		"https://api.openai.com/v1/embeddings": "https://api.openai.com/v1/embeddings",
		"http://127.0.0.1:11434/v1":            "http://127.0.0.1:11434/v1/embeddings",
		"  https://host/v1  ":                  "https://host/v1/embeddings",
	}
	for base, want := range cases {
		if got := externalEmbeddingsEndpoint(base); got != want {
			t.Errorf("externalEmbeddingsEndpoint(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestRetryableEmbeddingStatus(t *testing.T) {
	cases := map[int]bool{
		200: false, 400: false, 401: false, 403: false, 404: false,
		429: true, 500: true, 502: true, 503: true,
	}
	for status, want := range cases {
		if got := retryableEmbeddingStatus(status); got != want {
			t.Errorf("retryableEmbeddingStatus(%d) = %v, want %v", status, got, want)
		}
	}
}

func TestIsEmbeddingModel(t *testing.T) {
	cases := map[string]bool{
		"text-embedding-3-small": true,
		"bge-m3-Embed":           true,
		"nomic-embed-text":       true,
		"gpt-4o":                 false,
		"claude-opus-5":          false,
	}
	for id, want := range cases {
		if got := isEmbeddingModel(id); got != want {
			t.Errorf("isEmbeddingModel(%q) = %v, want %v", id, got, want)
		}
	}
}

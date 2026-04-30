package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-embedding-001:batchEmbedContents" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("x-goog-api-key = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		requests, ok := body["requests"].([]any)
		if !ok || len(requests) != 2 {
			t.Fatalf("requests = %#v", body["requests"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[1,2,3]},{"values":[4,5,6]}]}`))
	}))
	defer server.Close()

	client, err := New("test-key", WithEndpoint(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	vectors, err := client.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 3 || client.Dim() != 3 {
		t.Fatalf("vectors = %#v dim = %d", vectors, client.Dim())
	}
}

func TestClientEmbedWithOutputDimensionality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		requests := body["requests"].([]any)
		request := requests[0].(map[string]any)
		if request["outputDimensionality"] != float64(8) {
			t.Fatalf("outputDimensionality = %#v", request["outputDimensionality"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[1,2,3,4,5,6,7,8]}]}`))
	}))
	defer server.Close()

	client, err := New("test-key", WithEndpoint(server.URL), WithOutputDimensionality(8))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Embed(context.Background(), []string{"a"}); err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if client.Dim() != 8 {
		t.Fatalf("Dim() = %d", client.Dim())
	}
}

func TestClientEmbedStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	client, err := New("test-key", WithEndpoint(server.URL))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Embed(context.Background(), []string{"a"}); err == nil {
		t.Fatal("Embed() error = nil")
	}
}

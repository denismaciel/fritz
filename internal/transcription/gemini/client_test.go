package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fritz/internal/transcription"
)

func TestClientTranscribeUploadsAudioAndReturnsText(t *testing.T) {
	t.Helper()

	var sawUpload bool
	var sawGenerate bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/upload/v1beta/files" && r.Method == http.MethodPost:
			sawUpload = true
			w.Header().Set("x-goog-upload-url", server.URL+"/upload-session")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/upload-session" && r.Method == http.MethodPost:
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload body: %v", err)
			}
			if string(body) != "voice-bytes" {
				t.Fatalf("upload body = %q", string(body))
			}
			w.Header().Set("content-type", "application/json")
			_, _ = io.WriteString(w, `{"file":{"uri":"files/123","mimeType":"audio/ogg"}}`)
		case r.URL.Path == "/v1beta/models/gemini-3-flash-preview:generateContent" && r.Method == http.MethodPost:
			sawGenerate = true
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode generate payload: %v", err)
			}
			contents := payload["contents"].([]any)
			parts := contents[0].(map[string]any)["parts"].([]any)
			if parts[0].(map[string]any)["text"] == "" {
				t.Fatal("missing prompt text")
			}
			fileData := parts[1].(map[string]any)["file_data"].(map[string]any)
			if fileData["file_uri"] != "files/123" || fileData["mime_type"] != "audio/ogg" {
				t.Fatalf("file_data = %#v", fileData)
			}
			w.Header().Set("content-type", "application/json")
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"transcribed words"}]}}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", WithEndpoint(server.URL))
	text, err := client.Transcribe(context.Background(), transcription.AudioInput{
		Bytes:    []byte("voice-bytes"),
		MIMEType: "audio/ogg",
		FileName: "note.ogg",
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if text != "transcribed words" {
		t.Fatalf("text = %q", text)
	}
	if !sawUpload || !sawGenerate {
		t.Fatalf("sawUpload=%v sawGenerate=%v", sawUpload, sawGenerate)
	}
}

func TestClientTranscribeRejectsEmptyTranscript(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/upload/v1beta/files":
			w.Header().Set("x-goog-upload-url", server.URL+"/upload-session")
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/upload-session":
			w.Header().Set("content-type", "application/json")
			_, _ = io.WriteString(w, `{"file":{"uri":"files/123","mimeType":"audio/ogg"}}`)
		case strings.HasSuffix(r.URL.Path, ":generateContent"):
			w.Header().Set("content-type", "application/json")
			_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"   "} ]}}]}`)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", WithEndpoint(server.URL))
	_, err := client.Transcribe(context.Background(), transcription.AudioInput{
		Bytes:    []byte("voice-bytes"),
		MIMEType: "audio/ogg",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

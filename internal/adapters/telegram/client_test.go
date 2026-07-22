package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactTelegramError(t *testing.T) {
	err := redactTelegramError(errors.New(`Post "https://api.telegram.org/bot123456:secret/getUpdates": context canceled`))
	text := err.Error()
	if strings.Contains(text, "123456:secret") {
		t.Fatalf("token leaked: %s", text)
	}
	if !strings.Contains(text, "/botREDACTED/getUpdates") {
		t.Fatalf("redacted error = %s", text)
	}
}

func TestHTTPClientSetsBotCommands(t *testing.T) {
	var got SetMyCommandsRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/setMyCommands" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer server.Close()

	client := NewHTTPClient(server.Client(), server.URL, "token")
	err := client.SetMyCommands(context.Background(), SetMyCommandsRequest{Commands: []BotCommand{{
		Command: "training", Description: "Show today's training",
	}}})
	if err != nil {
		t.Fatalf("SetMyCommands() error = %v", err)
	}
	if len(got.Commands) != 1 || got.Commands[0].Command != "training" {
		t.Fatalf("commands = %#v", got.Commands)
	}
}

func TestHTTPClientDeletesWebhookWithoutDroppingUpdates(t *testing.T) {
	var got DeleteWebhookRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottoken/deleteWebhook" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": true})
	}))
	defer server.Close()

	client := NewHTTPClient(server.Client(), server.URL, "token")
	if err := client.DeleteWebhook(context.Background(), DeleteWebhookRequest{}); err != nil {
		t.Fatalf("DeleteWebhook() error = %v", err)
	}
	if got.DropPendingUpdates {
		t.Fatal("DeleteWebhook() dropped pending updates")
	}
}

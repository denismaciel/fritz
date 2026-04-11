package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/authstore"
	"fritz/internal/config"
	"fritz/internal/heartbeat"
	"fritz/internal/model"
	"fritz/internal/openaicodex"
	"fritz/internal/prompt"
	"fritz/internal/provider"
	"fritz/internal/tool"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fritz-app-tests-*")
	if err != nil {
		panic(err)
	}
	for _, name := range []string{
		"FRITZ_PROVIDER",
		"FRITZ_MODEL",
		"GEMINI_API_KEY",
		"FRITZ_GEMINI_ENDPOINT",
		"FRITZ_OPENAI_CODEX_ENDPOINT",
		"FRITZ_OPENAI_CODEX_AUTH_BASE_URL",
		"FRITZ_OPENAI_CODEX_CLIENT_ID",
		"FRITZ_OPENAI_CODEX_ORIGINATOR",
		"FRITZ_OPENAI_CODEX_REDIRECT_URL",
		"TELEGRAM_BOT_TOKEN",
		"FRITZ_TELEGRAM_PAIRING_TOKEN",
		"FRITZ_TELEGRAM_ALLOWED_USERS",
	} {
		_ = os.Unsetenv(name)
	}
	_ = os.Setenv("HOME", dir)
	_ = os.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	_ = os.Setenv("FRITZ_LOG_FILE", filepath.Join(dir, "agent.jsonl"))
	_ = os.Setenv("FRITZ_LOG_LEVEL", "debug")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func useAuthStore(t *testing.T, store authstore.Store) {
	t.Helper()
	prev := newAuthStore
	newAuthStore = func() authstore.Store {
		return store
	}
	t.Cleanup(func() {
		newAuthStore = prev
	})
}

func TestDefaultToolRegistryHasCodingTools(t *testing.T) {
	defs := defaultToolRegistry().Definitions()
	if len(defs) != 14 {
		t.Fatalf("Definitions() = %#v", defs)
	}
}

func TestRunHelp(t *testing.T) {
	output := captureStdout(t, func() {
		err := Run(context.Background(), []string{"help"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if !strings.Contains(output, "usage:") {
		t.Fatalf("expected usage output, got %q", output)
	}
	if !strings.Contains(output, "fritz doctor") {
		t.Fatalf("expected doctor command in output, got %q", output)
	}
}

func TestRunDoctorWithAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	output := captureStdout(t, func() {
		err := Run(context.Background(), []string{"doctor"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if !strings.Contains(output, "provider: gemini") {
		t.Fatalf("expected provider output, got %q", output)
	}
	if !strings.Contains(output, "GEMINI_API_KEY: set") {
		t.Fatalf("expected key status output, got %q", output)
	}
}

func TestRunDoctorOpenAICodex(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("FRITZ_PROVIDER", "openai-codex")
	useAuthStore(t, authstore.NewFileStore(t.TempDir()))

	output := captureStdout(t, func() {
		err := Run(context.Background(), []string{"doctor"})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	})

	if !strings.Contains(output, "provider: openai-codex") {
		t.Fatalf("expected provider output, got %q", output)
	}
	if !strings.Contains(output, "openai_codex_auth: missing") {
		t.Fatalf("expected codex auth status, got %q", output)
	}
	if !strings.Contains(output, "endpoint: https://chatgpt.com/backend-api") {
		t.Fatalf("expected codex endpoint, got %q", output)
	}
}

func TestRunDoctorWithoutAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")

	output := captureStdout(t, func() {
		err := Run(context.Background(), []string{"doctor"})
		if err == nil || err.Error() != "missing GEMINI_API_KEY" {
			t.Fatalf("expected error, got nil")
		}
	})

	if !strings.Contains(output, "GEMINI_API_KEY: missing") {
		t.Fatalf("expected missing key output, got %q", output)
	}
}

func TestRunDoctorUsesConfigFileAndFlags(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")
	t.Setenv("FRITZ_MODEL", "env-model")

	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".fritz")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"model":"file-model"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	var output bytes.Buffer
	err = run(context.Background(), []string{"--model", "flag-model", "doctor"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "model: flag-model") {
		t.Fatalf("output = %q", output.String())
	}
	if !strings.Contains(output.String(), "endpoint:") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunDoctorUsesProviderFlag(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")

	var output bytes.Buffer
	err := run(context.Background(), []string{"--provider", "openai-codex", "doctor"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "provider: openai-codex") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunAuthStatusWithStoredOAuth(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	store := authstore.NewFileStore(dir)
	if err := store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "access",
		RefreshToken: "refresh",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}
	useAuthStore(t, store)

	var output bytes.Buffer
	err = run(context.Background(), []string{"auth", "status", "openai-codex"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "provider: openai-codex") || !strings.Contains(output.String(), "status: oauth") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunAuthLogout(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	store := authstore.NewFileStore(dir)
	if err := store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "access",
		RefreshToken: "refresh",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}
	useAuthStore(t, store)

	var output bytes.Buffer
	err = run(context.Background(), []string{"auth", "logout", "openai-codex"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "auth removed: openai-codex") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestRunAuthLoginOpenAICodexManual(t *testing.T) {
	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	prevOpenBrowser := openBrowserURL
	prevCreateFlow := createOpenAICodexAuthorizationFlow
	prevStartServer := startOpenAICodexCallbackServer
	prevExchange := exchangeOpenAICodexAuthorizationCode
	createOpenAICodexAuthorizationFlow = func(openaicodex.OAuthConfig) (openaicodex.AuthorizationFlow, error) {
		return openaicodex.AuthorizationFlow{
			State:    "state-123",
			Verifier: "verifier-123",
			URL:      "https://auth.example.test/oauth/authorize?state=state-123",
		}, nil
	}
	openBrowserURL = func(context.Context, string) error { return nil }
	startOpenAICodexCallbackServer = func(openaicodex.OAuthConfig, string) (*openaicodex.CallbackServer, error) {
		return nil, errors.New("listen failed")
	}
	exchangeOpenAICodexAuthorizationCode = func(_ context.Context, _ openaicodex.OAuthConfig, code string, _ string) (authstore.OAuthCredential, error) {
		if code != "code-123" {
			t.Fatalf("code = %q", code)
		}
		return authstore.OAuthCredential{
			AccessToken:  "access",
			RefreshToken: "refresh",
			AccountID:    "acct_123",
			ExpiresAt:    time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC),
		}, nil
	}
	defer func() {
		createOpenAICodexAuthorizationFlow = prevCreateFlow
		openBrowserURL = prevOpenBrowser
		startOpenAICodexCallbackServer = prevStartServer
		exchangeOpenAICodexAuthorizationCode = prevExchange
	}()
	useAuthStore(t, authstore.NewFileStore(dir))

	var output bytes.Buffer
	err = run(
		context.Background(),
		[]string{"--provider", "openai-codex", "auth", "login", "openai-codex"},
		strings.NewReader("http://localhost:1455/auth/callback?code=code-123&state=state-123\n"),
		&output,
		func(_ config.Runtime) model.Client {
			t.Fatal("unexpected gateway creation")
			return nil
		},
		tool.NewRegistry(),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(output.String(), "auth stored: openai-codex") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestDefaultGatewayFactoryOpenAICodex(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token-123" {
			t.Fatalf("Authorization = %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acct_123" {
			t.Fatalf("chatgpt-account-id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
	}))
	defer server.Close()

	dir := t.TempDir()
	store := authstore.NewFileStore(dir)
	if err := store.PutOAuth(provider.OpenAICodex, authstore.OAuthCredential{
		AccessToken:  "token-123",
		RefreshToken: "refresh-123",
		AccountID:    "acct_123",
		ExpiresAt:    time.Date(2036, 4, 6, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutOAuth() error = %v", err)
	}
	useAuthStore(t, store)

	cfg := config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
		Flags: config.Source{
			Provider:            "openai-codex",
			OpenAICodexEndpoint: server.URL,
			ModelID:             "gpt-5.4",
		},
	})
	gateway := defaultClientFactory(dir, cfg)
	resp, err := gateway.Generate(context.Background(), model.Request{
		SystemPrompt: "sys",
		Messages:     []model.Message{model.TextMessage(model.UserRole, "hi")},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "hello" {
		t.Fatalf("Text = %q", resp.Text)
	}
}

func TestRunAgentWithPrompt(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	var streamCalls int

	err := run(context.Background(), []string{"run", "hi there"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		return fakeGateway{
			response:    "hello back",
			streamCalls: &streamCalls,
		}
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	text := output.String()
	if !strings.Contains(text, "hello back") {
		t.Fatalf("expected response output, got %q", text)
	}
	if streamCalls != 1 {
		t.Fatalf("expected stream call, got %d", streamCalls)
	}
}

func TestRunAgentWritesStructuredLogFile(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	dir := t.TempDir()
	logPath := filepath.Join(dir, "agent.jsonl")
	t.Setenv("FRITZ_LOG_FILE", logPath)
	t.Setenv("FRITZ_LOG_LEVEL", "debug")

	var output bytes.Buffer
	err := run(context.Background(), []string{"run", "hi there"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		return fakeGateway{response: "hello back"}
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"component":"app"`) || !strings.Contains(text, `"component":"agent"`) || !strings.Contains(text, `"event":"app.logger.configured"`) {
		t.Fatalf("log = %q", text)
	}
}

func TestRunAgentWithoutPrompt(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer

	err := run(context.Background(), []string{"run"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err == nil || err.Error() != "command error: missing prompt" {
		t.Fatalf("expected missing prompt error, got %v", err)
	}
}

func TestRunAgentRejectsChatSubcommand(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer

	err := run(context.Background(), []string{"run", "chat"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err == nil || err.Error() != `command error: use "fritz chat" for interactive mode` {
		t.Fatalf("expected chat guidance error, got %v", err)
	}
}

func TestRunChatLoop(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	var lastTexts []string
	var streamCalls int
	var generateCalls int

	err := run(
		context.Background(),
		[]string{"chat"},
		strings.NewReader("hi\nwhat now\n:quit\n"),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				texts:         &lastTexts,
				streamCalls:   &streamCalls,
				generateCalls: &generateCalls,
				responseForRequest: func(_ model.Request, call int) string {
					return fmt.Sprintf("resp %d", call)
				},
			}
		},
		tool.NewRegistry(),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if len(lastTexts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(lastTexts))
	}
	if lastTexts[0] != "hi" {
		t.Fatalf("first prompt = %q", lastTexts[0])
	}
	if lastTexts[1] != "what now" {
		t.Fatalf("second prompt = %q", lastTexts[1])
	}
	if !strings.Contains(output.String(), "resp 1") || !strings.Contains(output.String(), "resp 2") {
		t.Fatalf("expected chat responses in output, got %q", output.String())
	}
	if streamCalls != 2 {
		t.Fatalf("expected 2 stream calls, got %d", streamCalls)
	}
}

func TestRunNoArgsDefaultsToChat(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	var streamCalls int

	err := run(
		context.Background(),
		nil,
		strings.NewReader(":quit\n"),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{streamCalls: &streamCalls}
		},
		tool.NewRegistry(),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if streamCalls != 0 {
		t.Fatalf("expected no model call on immediate quit, got %d", streamCalls)
	}
}

func TestRunChatHelpAndReset(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	var lastTexts []string
	var generateCalls int

	err := run(
		context.Background(),
		[]string{"chat"},
		strings.NewReader(":help\nhi\n:reset\nagain\n:quit\n"),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				texts:         &lastTexts,
				response:      "ok",
				generateCalls: &generateCalls,
			}
		},
		tool.NewRegistry(),
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if len(lastTexts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(lastTexts))
	}
	if lastTexts[0] != "hi" {
		t.Fatalf("first prompt = %q", lastTexts[0])
	}
	if lastTexts[1] != "again" {
		t.Fatalf("second prompt after reset = %q", lastTexts[1])
	}
	if !strings.Contains(output.String(), ":reset") {
		t.Fatalf("expected help output, got %q", output.String())
	}
	if !strings.Contains(output.String(), "history cleared") {
		t.Fatalf("expected reset output, got %q", output.String())
	}
}

func TestRunAgentToolLoopRead(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("hello from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := tool.NewRegistry()
	registry.Register(tool.NewReadTool(dir, 1024))

	var output bytes.Buffer
	var sawToolResult bool
	var generateCalls int

	err := run(
		context.Background(),
		[]string{"run", "summarize README"},
		strings.NewReader(""),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				streamDisabled: true,
				generateCalls:  &generateCalls,
				generateFunc: func(req model.Request, call int) model.Response {
					if call == 1 {
						return model.Response{
							Message: model.Message{
								Role: model.ModelRole,
								Parts: []model.Part{
									{
										ToolCall: &tool.Call{
											ID:   "call-1",
											Name: "read",
											Args: map[string]any{"path": "README.md"},
										},
									},
								},
							},
							ToolCalls: []tool.Call{
								{ID: "call-1", Name: "read", Args: map[string]any{"path": "README.md"}},
							},
						}
					}
					for _, message := range req.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && strings.Contains(part.ToolResult.Text(), "hello from file") {
								sawToolResult = true
							}
						}
					}
					return model.Response{
						Message: model.TextMessage(model.ModelRole, "done"),
						Text:    "done",
					}
				},
			}
		},
		registry,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !sawToolResult {
		t.Fatal("expected tool result content in follow-up request")
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("expected final answer, got %q", output.String())
	}
}

func TestRunAgentPersistsAndContinuesSession(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	dir := t.TempDir()
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() {
		_ = os.Chdir(origWD)
	}()

	var firstOutput bytes.Buffer
	err = run(context.Background(), []string{"run", "hello"}, strings.NewReader(""), &firstOutput, func(_ config.Runtime) model.Client {
		return fakeGateway{response: "world"}
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("first run() error = %v", err)
	}

	sessionRoot := filepath.Join(dir, ".fritz", "sessions")
	var sessionFiles []string
	_ = filepath.WalkDir(sessionRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		sessionFiles = append(sessionFiles, path)
		return nil
	})
	if len(sessionFiles) != 1 {
		t.Fatalf("sessionFiles = %#v", sessionFiles)
	}

	var sawPrior bool
	var secondOutput bytes.Buffer
	err = run(context.Background(), []string{"--continue", "run", "again"}, strings.NewReader(""), &secondOutput, func(_ config.Runtime) model.Client {
		return fakeGateway{
			response: "done",
			generateFunc: func(req model.Request, _ int) model.Response {
				for _, message := range req.Messages {
					if strings.Contains(message.Text(), "hello") || strings.Contains(message.Text(), "world") {
						sawPrior = true
					}
				}
				return model.Response{
					Message: model.TextMessage(model.ModelRole, "done"),
					Text:    "done",
				}
			},
		}
	}, tool.NewRegistry())
	if err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if !sawPrior {
		t.Fatal("expected prior session context")
	}
}

func TestRunAgentBuildsSystemPromptAndExpandsSkill(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("be terse"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "memory"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("persist this fact"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".fritz"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".fritz", "secrets.json"), []byte("{\n  \"version\": 1,\n  \"secrets\": {\n    \"strava.api_key\": {\"value\": \"super-secret\"}\n  }\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	skillDir := filepath.Join(dir, ".fritz", "skills", "task-pack-create")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
description: Create task packs
---

Use this skill.`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	var gotRequest model.Request
	var output bytes.Buffer
	err = run(
		context.Background(),
		[]string{"run", "/skill:task-pack-create build it"},
		strings.NewReader(""),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				generateFunc: func(req model.Request, _ int) model.Response {
					gotRequest = req
					return model.Response{
						Message: model.TextMessage(model.ModelRole, "done"),
						Text:    "done",
					}
				},
			}
		},
		nil,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !strings.Contains(gotRequest.SystemPrompt, "be terse") {
		t.Fatalf("SystemPrompt = %q", gotRequest.SystemPrompt)
	}
	if strings.Contains(gotRequest.SystemPrompt, "persist this fact") {
		t.Fatalf("plain SystemPrompt leaked memory = %q", gotRequest.SystemPrompt)
	}
	if strings.Contains(gotRequest.SystemPrompt, "super-secret") {
		t.Fatalf("SystemPrompt leaked secret = %q", gotRequest.SystemPrompt)
	}
	if strings.Contains(gotRequest.SystemPrompt, "# Durable Memory") || strings.Contains(gotRequest.SystemPrompt, "# Heartbeat Context") {
		t.Fatalf("plain SystemPrompt leaked gateway sections = %q", gotRequest.SystemPrompt)
	}
	if !strings.Contains(gotRequest.SystemPrompt, "task-pack-create") {
		t.Fatalf("SystemPrompt = %q", gotRequest.SystemPrompt)
	}
	if !strings.Contains(gotRequest.Messages[len(gotRequest.Messages)-1].Text(), `<skill name="task-pack-create"`) {
		t.Fatalf("Message = %#v", gotRequest.Messages[len(gotRequest.Messages)-1])
	}
}

func TestRunTelegramPollOnce(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	sessionDir := t.TempDir()

	var sentText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botbot-token/getUpdates":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": []map[string]any{{
					"update_id": 1,
					"message": map[string]any{
						"message_id": 10,
						"chat": map[string]any{
							"id":   42,
							"type": "private",
						},
						"from": map[string]any{
							"id": 7,
						},
						"text": "hi",
					},
				}},
			})
		case "/botbot-token/sendMessage":
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			sentText = payload.Text
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := runWithProfile(
		context.Background(),
		[]string{
			"--telegram-endpoint", server.URL,
			"--telegram-poll-timeout", "1s",
			"--telegram-allow-user", "7",
			"--session-dir", sessionDir,
			"telegram",
			"--poll-once",
		},
		strings.NewReader(""),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{response: "pong"}
		},
		tool.NewRegistry(),
		prompt.ProfileCoding,
		true,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if sentText != "pong" {
		t.Fatalf("sentText = %q", sentText)
	}
}

func TestRunTelegramHeartbeatPollOnce(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("TELEGRAM_BOT_TOKEN", "bot-token")
	sessionDir := t.TempDir()

	origSource := newHeartbeatSource
	newHeartbeatSource = func(config.Runtime) heartbeat.Source {
		return heartbeatSourceFunc(func(context.Context, time.Time) ([]heartbeat.Wake, error) {
			return []heartbeat.Wake{{
				TargetKey: "telegram:dm:7",
				Channel:   "telegram",
				ChatID:    "42",
				UserID:    "7",
				Reason:    "tick",
			}}, nil
		})
	}
	defer func() { newHeartbeatSource = origSource }()

	var sent []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/botbot-token/getUpdates":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": []any{}})
		case "/botbot-token/sendMessage":
			var payload struct {
				Text string `json:"text"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			sent = append(sent, payload.Text)
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{}})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := runWithProfile(
		context.Background(),
		[]string{
			"--telegram-endpoint", server.URL,
			"--heartbeat=true",
			"--heartbeat-interval", "1s",
			"--session-dir", sessionDir,
			"telegram",
			"--poll-once",
		},
		strings.NewReader(""),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				generateFunc: func(req model.Request, _ int) model.Response {
					text := req.Messages[len(req.Messages)-1].Text()
					if strings.Contains(text, "Heartbeat check") || strings.Contains(text, "reminder or scheduled task is due now") {
						return model.Response{
							Message: model.TextMessage(model.ModelRole, "heartbeat ping"),
							Text:    "heartbeat ping",
						}
					}
					return model.Response{
						Message: model.TextMessage(model.ModelRole, "other"),
						Text:    "other",
					}
				},
			}
		},
		tool.NewRegistry(),
		prompt.ProfileCoding,
		true,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(sent) != 1 || sent[0] != "heartbeat ping" {
		t.Fatalf("sent = %#v", sent)
	}
}

func TestRunAgentToolLoopStream(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	registry := tool.NewRegistry()
	registry.Register(tool.NewReadTool(t.TempDir(), 1024))

	var output bytes.Buffer
	var generateCalls int
	var streamCalls int
	var sawToolResult bool

	err := run(
		context.Background(),
		[]string{"run", "summarize README"},
		strings.NewReader(""),
		&output,
		func(_ config.Runtime) model.Client {
			return fakeGateway{
				generateCalls: &generateCalls,
				streamCalls:   &streamCalls,
				generateFunc: func(req model.Request, call int) model.Response {
					if call == 1 {
						return model.Response{
							Message: model.Message{
								Role: model.ModelRole,
								Parts: []model.Part{
									{
										Text: "I'll read the file.",
									},
									{
										ToolCall: &tool.Call{
											ID:   "call-1",
											Name: "read",
											Args: map[string]any{"path": "README.md"},
										},
									},
								},
							},
							Text: "I'll read the file.",
							ToolCalls: []tool.Call{
								{ID: "call-1", Name: "read", Args: map[string]any{"path": "README.md"}},
							},
						}
					}
					// Verify that the second call contains the tool result
					for _, message := range req.Messages {
						for _, part := range message.Parts {
							if part.ToolResult != nil && part.ToolResult.Name == "read" {
								sawToolResult = true
							}
						}
					}
					return model.Response{
						Message: model.TextMessage(model.ModelRole, "done"),
						Text:    "done",
					}
				},
			}
		},
		registry,
	)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if generateCalls != 2 {
		t.Errorf("expected 2 generate calls, got %d", generateCalls)
	}
	if !sawToolResult {
		t.Error("expected tool result in the second model call")
	}
}

func TestRunWrapsConfigErrors(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	err := run(context.Background(), []string{"--config", "/missing/config.json", "doctor"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "config error:") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunWrapsPromptErrors(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	var output bytes.Buffer
	err := run(context.Background(), []string{"run", "/skill:nope"}, strings.NewReader(""), &output, func(_ config.Runtime) model.Client {
		t.Fatal("unexpected gateway creation")
		return nil
	}, tool.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), `prompt error: unknown skill "nope"`) {
		t.Fatalf("error = %v", err)
	}
}

type fakeGateway struct {
	response           string
	err                error
	texts              *[]string
	responseForRequest func(req model.Request, call int) string
	generateFunc       func(req model.Request, call int) model.Response
	streamDeltas       []string
	streamCalls        *int
	streamDisabled     bool
	generateCalls      *int
}

type heartbeatSourceFunc func(context.Context, time.Time) ([]heartbeat.Wake, error)

func (f heartbeatSourceFunc) Due(ctx context.Context, now time.Time) ([]heartbeat.Wake, error) {
	return f(ctx, now)
}

func (f fakeGateway) Generate(_ context.Context, req model.Request) (model.Response, error) {
	if f.err != nil {
		return model.Response{}, f.err
	}
	call := 1
	if f.generateCalls != nil {
		*f.generateCalls++
		call = *f.generateCalls
	}
	if f.texts != nil && len(req.Messages) > 0 {
		*f.texts = append(*f.texts, req.Messages[len(req.Messages)-1].Text())
	}
	if f.generateFunc != nil {
		return f.generateFunc(req, call), nil
	}
	if f.responseForRequest != nil {
		text := f.responseForRequest(req, call)
		return model.Response{
			Message: model.TextMessage(model.ModelRole, text),
			Text:    text,
		}, nil
	}
	return model.Response{
		Message: model.TextMessage(model.ModelRole, f.response),
		Text:    f.response,
	}, nil
}

func (f fakeGateway) StreamGenerate(ctx context.Context, req model.Request, emit func(model.StreamEvent) error) (model.Response, error) {
	if f.streamDisabled {
		return model.Response{}, fmt.Errorf("stream disabled")
	}
	if f.streamCalls != nil {
		*f.streamCalls++
	}
	resp, err := f.Generate(ctx, req)
	if err != nil {
		return model.Response{}, err
	}
	deltas := f.streamDeltas
	if len(deltas) == 0 && resp.Text != "" {
		deltas = []string{resp.Text}
	}
	for _, delta := range deltas {
		if err := emit(model.StreamEvent{TextDelta: delta}); err != nil {
			return model.Response{}, err
		}
	}
	return resp, nil
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
	}()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	return <-done
}

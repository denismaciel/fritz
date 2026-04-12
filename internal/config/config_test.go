package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestResolveMergesSources(t *testing.T) {
	runtime := Resolve(Sources{
		Defaults: DefaultSource(),
		File: Source{
			Provider: "gemini",
			ModelID:  "file-model",
			Log: LogConfigSource{
				File:  "file-log.jsonl",
				Level: "warn",
			},
			Telegram: TelegramConfigSource{
				Endpoint: "https://telegram.file.test",
			},
			Heartbeat: HeartbeatConfigSource{
				Enabled: boolPtr(true),
			},
			Chat: ChatConfigSource{
				ShowHelpOnStart: boolPtr(false),
			},
			Prompt: PromptConfigSource{
				SkillPaths: []string{"/file-skill"},
			},
		},
		Env: Source{
			Provider:     "gemini",
			GeminiAPIKey: "env-key",
			Log: LogConfigSource{
				File: "env-log.jsonl",
			},
			Telegram: TelegramConfigSource{
				BotToken: "env-bot-token",
			},
			Session: SessionConfigSource{
				Dir:                    "/tmp/sessions",
				CompactThresholdTokens: intPtr(9000),
			},
			Prompt: PromptConfigSource{
				SkillPaths: []string{"/env-skill"},
			},
		},
		Flags: Source{
			Provider: "openai-codex",
			ModelID:  "flag-model",
			Log: LogConfigSource{
				Level: "debug",
			},
			OpenAICodexEndpoint:    "https://chatgpt.flag.test/backend-api",
			OpenAICodexAuthBaseURL: "https://auth.flag.test",
			OpenAICodexClientID:    "client-flag",
			OpenAICodexOriginator:  "fritz-test",
			OpenAICodexRedirectURL: "http://localhost:2455/auth/callback",
			Telegram: TelegramConfigSource{
				PairingToken: "flag-pair",
				AllowedUsers: []string{"7", "8"},
			},
			Heartbeat: HeartbeatConfigSource{
				Interval: durationPtr(2 * time.Minute),
			},
			Runtime: RuntimeConfigSource{
				CommandTimeout: durationPtr(5 * time.Second),
			},
			Prompt: PromptConfigSource{
				NoSkills:   boolPtr(true),
				SkillPaths: []string{"/flag-skill"},
			},
			Session: SessionConfigSource{
				CompactTargetTokens: intPtr(3000),
			},
		},
	})

	if runtime.Provider != "openai-codex" {
		t.Fatalf("Provider = %q", runtime.Provider)
	}
	if runtime.GeminiAPIKey != "env-key" {
		t.Fatalf("GeminiAPIKey = %q", runtime.GeminiAPIKey)
	}
	if runtime.ModelID != "flag-model" {
		t.Fatalf("ModelID = %q", runtime.ModelID)
	}
	if runtime.Telegram.BotToken != "env-bot-token" {
		t.Fatalf("Telegram.BotToken = %q", runtime.Telegram.BotToken)
	}
	if runtime.Telegram.Endpoint != "https://telegram.file.test" {
		t.Fatalf("Telegram.Endpoint = %q", runtime.Telegram.Endpoint)
	}
	if runtime.Log.File != "env-log.jsonl" || runtime.Log.Level != "debug" {
		t.Fatalf("Log = %#v", runtime.Log)
	}
	if runtime.Telegram.PairingToken != "flag-pair" {
		t.Fatalf("Telegram.PairingToken = %q", runtime.Telegram.PairingToken)
	}
	if !reflect.DeepEqual(runtime.Telegram.AllowedUsers, []string{"7", "8"}) {
		t.Fatalf("Telegram.AllowedUsers = %#v", runtime.Telegram.AllowedUsers)
	}
	if !runtime.Heartbeat.Enabled || runtime.Heartbeat.Interval != 2*time.Minute {
		t.Fatalf("Heartbeat = %#v", runtime.Heartbeat)
	}
	if runtime.Chat.ShowHelpOnStart {
		t.Fatal("expected file override for chat help")
	}
	if runtime.Session.Dir != "/tmp/sessions" {
		t.Fatalf("Session.Dir = %q", runtime.Session.Dir)
	}
	if runtime.Session.CompactThresholdTokens != 9000 || runtime.Session.CompactTargetTokens != 3000 {
		t.Fatalf("Session = %#v", runtime.Session)
	}
	if runtime.CommandTimeout != 5*time.Second {
		t.Fatalf("CommandTimeout = %s", runtime.CommandTimeout)
	}
	if runtime.OpenAICodexEndpoint != "https://chatgpt.flag.test/backend-api" {
		t.Fatalf("OpenAICodexEndpoint = %q", runtime.OpenAICodexEndpoint)
	}
	if runtime.OpenAICodexAuthBaseURL != "https://auth.flag.test" {
		t.Fatalf("OpenAICodexAuthBaseURL = %q", runtime.OpenAICodexAuthBaseURL)
	}
	if runtime.OpenAICodexClientID != "client-flag" {
		t.Fatalf("OpenAICodexClientID = %q", runtime.OpenAICodexClientID)
	}
	if runtime.OpenAICodexOriginator != "fritz-test" {
		t.Fatalf("OpenAICodexOriginator = %q", runtime.OpenAICodexOriginator)
	}
	if runtime.OpenAICodexRedirectURL != "http://localhost:2455/auth/callback" {
		t.Fatalf("OpenAICodexRedirectURL = %q", runtime.OpenAICodexRedirectURL)
	}
	if !runtime.Prompt.NoSkills {
		t.Fatal("expected no-skills override")
	}
	if len(runtime.Prompt.SkillPaths) != 3 {
		t.Fatalf("Prompt.SkillPaths = %#v", runtime.Prompt.SkillPaths)
	}
}

func TestRuntimeValidate(t *testing.T) {
	runtime := Resolve(Sources{Defaults: DefaultSource()})
	if err := runtime.Validate(); err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestResolveDefaultsModelForProvider(t *testing.T) {
	gemini := Resolve(Sources{Defaults: DefaultSource()})
	if gemini.ModelID != DefaultGeminiModelID {
		t.Fatalf("gemini ModelID = %q", gemini.ModelID)
	}

	openai := Resolve(Sources{
		Defaults: DefaultSource(),
		Flags: Source{
			Provider: "openai-codex",
		},
	})
	if openai.ModelID != DefaultOpenAICodexModelID {
		t.Fatalf("openai ModelID = %q", openai.ModelID)
	}
}

func TestRuntimeValidateOpenAICodexDoesNotRequireGeminiKey(t *testing.T) {
	runtime := Resolve(Sources{
		Defaults: DefaultSource(),
		Flags: Source{
			Provider: "openai-codex",
		},
	})
	if err := runtime.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRuntimeValidateRejectsOpenAIModelOnGemini(t *testing.T) {
	runtime := Resolve(Sources{
		Defaults: DefaultSource(),
		Env: Source{
			GeminiAPIKey: "env-key",
		},
		Flags: Source{
			Provider: "gemini",
			ModelID:  "gpt-5.4-high-reasoning",
		},
	})
	if err := runtime.Validate(); err == nil {
		t.Fatal("expected provider/model mismatch error")
	}
}

func TestRuntimeValidateRejectsGeminiModelOnOpenAI(t *testing.T) {
	runtime := Resolve(Sources{
		Defaults: DefaultSource(),
		Flags: Source{
			Provider: "openai-codex",
			ModelID:  "gemini-3-flash-preview",
		},
	})
	if err := runtime.Validate(); err == nil {
		t.Fatal("expected provider/model mismatch error")
	}
}

func TestRuntimeValidateRejectsUnknownProvider(t *testing.T) {
	runtime := Resolve(Sources{
		Defaults: DefaultSource(),
		Flags: Source{
			Provider: "nope",
		},
	})
	if err := runtime.Validate(); err == nil {
		t.Fatal("expected invalid provider error")
	}
}

func TestResolveStateHelpers(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/state")
	cwd := "/tmp/work"
	if got := GlobalStateRoot(); got != "/tmp/state/fritz" {
		t.Fatalf("GlobalStateRoot() = %q", got)
	}
	if got := WorkspaceStateRoot(cwd); got != "/tmp/state/fritz/workspaces/--tmp--work" {
		t.Fatalf("WorkspaceStateRoot() = %q", got)
	}
	if got := ResolveLogFile(cwd, ""); got != "/tmp/state/fritz/workspaces/--tmp--work/logs/agent.jsonl" {
		t.Fatalf("ResolveLogFile() = %q", got)
	}
	dir, err := ResolveSessionDir(cwd, "")
	if err != nil {
		t.Fatalf("ResolveSessionDir() error = %v", err)
	}
	if dir != "/tmp/state/fritz/workspaces/--tmp--work/sessions" {
		t.Fatalf("ResolveSessionDir() = %q", dir)
	}
}

func TestRuntimeValidateTelegram(t *testing.T) {
	runtime := Resolve(Sources{Defaults: DefaultSource()})
	if err := runtime.ValidateTelegram(); err == nil {
		t.Fatal("expected missing telegram token error")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{
  "provider": "openai-codex",
  "model": "file-model",
  "geminiEndpoint": "https://example.test",
  "openAICodexEndpoint": "https://chatgpt.example.test/backend-api",
  "openAICodexAuthBaseURL": "https://auth.example.test",
  "openAICodexClientID": "client-test",
  "openAICodexOriginator": "fritz-test",
  "openAICodexRedirectURL": "http://localhost:2455/auth/callback",
  "telegram": {
    "botToken": "bot-token",
    "endpoint": "https://telegram.test",
    "pollTimeout": "21s",
    "pairingToken": "secret",
    "allowedUsers": ["7", "8"]
  },
  "heartbeat": {
    "enabled": true,
    "interval": "2m"
  },
  "log": {
    "file": ".fritz/logs/custom.jsonl",
    "level": "debug"
  },
  "chat": {"showHelpOnStart": false},
  "session": {
    "enabled": false,
    "dir": ".sessions-alt",
    "autoCompact": false,
    "compactThresholdTurns": 12,
    "compactKeepTurns": 4,
    "compactThresholdTokens": 4096,
    "compactTargetTokens": 2048
  },
  "runtime": {"commandTimeout": "45s"},
  "prompt": {
    "noSkills": true,
    "skillPaths": ["./skills", "../more-skills"]
  }
}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if source.ModelID != "file-model" {
		t.Fatalf("ModelID = %q", source.ModelID)
	}
	if source.Provider != "openai-codex" {
		t.Fatalf("Provider = %q", source.Provider)
	}
	if source.GeminiEndpoint != "https://example.test" {
		t.Fatalf("GeminiEndpoint = %q", source.GeminiEndpoint)
	}
	if source.OpenAICodexEndpoint != "https://chatgpt.example.test/backend-api" {
		t.Fatalf("OpenAICodexEndpoint = %q", source.OpenAICodexEndpoint)
	}
	if source.OpenAICodexAuthBaseURL != "https://auth.example.test" {
		t.Fatalf("OpenAICodexAuthBaseURL = %q", source.OpenAICodexAuthBaseURL)
	}
	if source.OpenAICodexClientID != "client-test" {
		t.Fatalf("OpenAICodexClientID = %q", source.OpenAICodexClientID)
	}
	if source.OpenAICodexOriginator != "fritz-test" {
		t.Fatalf("OpenAICodexOriginator = %q", source.OpenAICodexOriginator)
	}
	if source.OpenAICodexRedirectURL != "http://localhost:2455/auth/callback" {
		t.Fatalf("OpenAICodexRedirectURL = %q", source.OpenAICodexRedirectURL)
	}
	if source.Telegram.BotToken != "bot-token" {
		t.Fatalf("Telegram.BotToken = %q", source.Telegram.BotToken)
	}
	if source.Telegram.Endpoint != "https://telegram.test" {
		t.Fatalf("Telegram.Endpoint = %q", source.Telegram.Endpoint)
	}
	if source.Telegram.PollTimeout == nil || *source.Telegram.PollTimeout != 21*time.Second {
		t.Fatalf("Telegram.PollTimeout = %#v", source.Telegram.PollTimeout)
	}
	if source.Telegram.PairingToken != "secret" {
		t.Fatalf("Telegram.PairingToken = %q", source.Telegram.PairingToken)
	}
	if !reflect.DeepEqual(source.Telegram.AllowedUsers, []string{"7", "8"}) {
		t.Fatalf("Telegram.AllowedUsers = %#v", source.Telegram.AllowedUsers)
	}
	if source.Heartbeat.Enabled == nil || !*source.Heartbeat.Enabled {
		t.Fatalf("Heartbeat.Enabled = %#v", source.Heartbeat.Enabled)
	}
	if source.Heartbeat.Interval == nil || *source.Heartbeat.Interval != 2*time.Minute {
		t.Fatalf("Heartbeat.Interval = %#v", source.Heartbeat.Interval)
	}
	if source.Log.File != ".fritz/logs/custom.jsonl" || source.Log.Level != "debug" {
		t.Fatalf("Log = %#v", source.Log)
	}
	if source.Chat.ShowHelpOnStart == nil || *source.Chat.ShowHelpOnStart {
		t.Fatalf("ShowHelpOnStart = %#v", source.Chat.ShowHelpOnStart)
	}
	if source.Session.Enabled == nil || *source.Session.Enabled {
		t.Fatalf("Session.Enabled = %#v", source.Session.Enabled)
	}
	if source.Session.Dir != ".sessions-alt" {
		t.Fatalf("Session.Dir = %q", source.Session.Dir)
	}
	if source.Session.AutoCompact == nil || *source.Session.AutoCompact {
		t.Fatalf("Session.AutoCompact = %#v", source.Session.AutoCompact)
	}
	if source.Session.CompactThresholdTurns == nil || *source.Session.CompactThresholdTurns != 12 {
		t.Fatalf("Session.CompactThresholdTurns = %#v", source.Session.CompactThresholdTurns)
	}
	if source.Session.CompactKeepTurns == nil || *source.Session.CompactKeepTurns != 4 {
		t.Fatalf("Session.CompactKeepTurns = %#v", source.Session.CompactKeepTurns)
	}
	if source.Session.CompactThresholdTokens == nil || *source.Session.CompactThresholdTokens != 4096 {
		t.Fatalf("Session.CompactThresholdTokens = %#v", source.Session.CompactThresholdTokens)
	}
	if source.Session.CompactTargetTokens == nil || *source.Session.CompactTargetTokens != 2048 {
		t.Fatalf("Session.CompactTargetTokens = %#v", source.Session.CompactTargetTokens)
	}
	if source.Runtime.CommandTimeout == nil || *source.Runtime.CommandTimeout != 45*time.Second {
		t.Fatalf("CommandTimeout = %#v", source.Runtime.CommandTimeout)
	}
	if source.Prompt.NoSkills == nil || !*source.Prompt.NoSkills {
		t.Fatalf("Prompt.NoSkills = %#v", source.Prompt.NoSkills)
	}
	if len(source.Prompt.SkillPaths) != 2 {
		t.Fatalf("Prompt.SkillPaths = %#v", source.Prompt.SkillPaths)
	}
}

func TestLoadForDirUsesDefaultPath(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".fritz")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"model":"dir-model"}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	source, path, err := LoadForDir(dir, "")
	if err != nil {
		t.Fatalf("LoadForDir() error = %v", err)
	}
	if path != filepath.Join(cfgDir, "config.json") {
		t.Fatalf("path = %q", path)
	}
	if source.ModelID != "dir-model" {
		t.Fatalf("ModelID = %q", source.ModelID)
	}
}

func TestLoadForDirMissingDefaultIsEmpty(t *testing.T) {
	dir := t.TempDir()

	source, path, err := LoadForDir(dir, "")
	if err != nil {
		t.Fatalf("LoadForDir() error = %v", err)
	}
	if path != "" {
		t.Fatalf("path = %q", path)
	}
	if len(source.Prompt.SkillPaths) != 0 {
		t.Fatalf("source = %#v", source)
	}
	source.Prompt.SkillPaths = nil
	if !reflect.DeepEqual(source, Source{}) {
		t.Fatalf("source = %#v", source)
	}
}

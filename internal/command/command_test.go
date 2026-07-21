package command

import (
	"reflect"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want any
	}{
		{
			name: "empty args becomes chat",
			args: nil,
			want: Chat{},
		},
		{
			name: "flags only becomes chat",
			args: []string{"--continue"},
			want: Chat{
				Session: SessionOptions{
					Continue: true,
				},
			},
		},
		{
			name: "explicit help",
			args: []string{"help"},
			want: Help{},
		},
		{
			name: "dash help",
			args: []string{"-h"},
			want: Help{},
		},
		{
			name: "doctor",
			args: []string{"doctor"},
			want: Doctor{},
		},
		{
			name: "chat",
			args: []string{"chat"},
			want: Chat{},
		},
		{
			name: "serve",
			args: []string{"serve"},
			want: Serve{},
		},
		{
			name: "attach",
			args: []string{"attach", "run-000001"},
			want: Attach{RunID: "run-000001"},
		},
		{
			name: "telegram",
			args: []string{"telegram"},
			want: Telegram{},
		},
		{
			name: "auth login",
			args: []string{"auth", "login", "openai-codex"},
			want: AuthLogin{Provider: "openai-codex"},
		},
		{
			name: "auth logout",
			args: []string{"auth", "logout", "openai-codex"},
			want: AuthLogout{Provider: "openai-codex"},
		},
		{
			name: "auth status",
			args: []string{"auth", "status", "openai-codex"},
			want: AuthStatus{Provider: "openai-codex"},
		},
		{
			name: "telegram poll once",
			args: []string{"telegram", "--poll-once"},
			want: Telegram{PollOnce: true},
		},
		{
			name: "run prompt",
			args: []string{"run", "hi", "there"},
			want: Run{Prompt: "hi there"},
		},
		{
			name: "session flags",
			args: []string{"--continue", "--session", "/tmp/a.jsonl", "--fork", "/tmp/b.jsonl", "chat"},
			want: Chat{
				Session: SessionOptions{
					Continue: true,
					Session:  "/tmp/a.jsonl",
					Fork:     "/tmp/b.jsonl",
				},
			},
		},
		{
			name: "config flags",
			args: []string{
				"--config", "/tmp/config.json",
				"--provider", "openai-codex",
				"--model", "flag-model",
				"--gemini-endpoint", "https://example.test",
				"--openai-codex-endpoint", "https://chatgpt.example.test/backend-api",
				"--openai-codex-auth-base-url", "https://auth.example.test",
				"--openai-codex-client-id", "client-123",
				"--openai-codex-originator", "fritz-test",
				"--openai-codex-redirect-url", "http://localhost:2455/auth/callback",
				"--telegram-endpoint", "https://telegram.example.test",
				"--telegram-poll-timeout", "25s",
				"--telegram-pairing-token", "pair-secret",
				"--telegram-allow-user", "7",
				"--telegram-allow-user", "8",
				"--telegram-training-db", "/data/training.db",
				"--heartbeat=true",
				"--heartbeat-interval", "2m",
				"--log-file", "/tmp/agent.jsonl",
				"--log-level", "debug",
				"--session-dir", "/tmp/sessions",
				"--chat-help=false",
				"--auto-compact=false",
				"--compact-threshold", "11",
				"--compact-keep", "5",
				"--compact-threshold-tokens", "4096",
				"--compact-target-tokens", "2048",
				"--command-timeout", "45s",
				"--skill", "/tmp/skill-a",
				"--skill", "/tmp/skill-b",
				"--no-skills",
				"--server", "http://127.0.0.1:8080",
				"--observe-socket", "/tmp/fritz.sock",
				"--gateway-session", "telegram:dm:7",
				"--listen", ":9000",
				"doctor",
			},
			want: Doctor{
				Config: ConfigOptions{
					Path:                   "/tmp/config.json",
					Provider:               "openai-codex",
					ModelID:                "flag-model",
					GeminiEndpoint:         "https://example.test",
					OpenAICodexEndpoint:    "https://chatgpt.example.test/backend-api",
					OpenAICodexAuthBaseURL: "https://auth.example.test",
					OpenAICodexClientID:    "client-123",
					OpenAICodexOriginator:  "fritz-test",
					OpenAICodexRedirectURL: "http://localhost:2455/auth/callback",
					TelegramEndpoint:       "https://telegram.example.test",
					TelegramPollTimeout:    durationPtr(25 * time.Second),
					TelegramPairingToken:   "pair-secret",
					TelegramAllowedUsers:   []string{"7", "8"},
					TelegramTrainingDBPath: "/data/training.db",
					HeartbeatEnabled:       boolPtr(true),
					HeartbeatInterval:      durationPtr(2 * time.Minute),
					LogFile:                "/tmp/agent.jsonl",
					LogLevel:               "debug",
					SessionDir:             "/tmp/sessions",
					ChatHelp:               boolPtr(false),
					AutoCompact:            boolPtr(false),
					CompactThresholdTurns:  intPtr(11),
					CompactKeepTurns:       intPtr(5),
					CompactThresholdTokens: intPtr(4096),
					CompactTargetTokens:    intPtr(2048),
					CommandTimeout:         durationPtr(45 * time.Second),
					SkillPaths:             []string{"/tmp/skill-a", "/tmp/skill-b"},
					NoSkills:               boolPtr(true),
					ServerURL:              "http://127.0.0.1:8080",
					ObserveSocket:          "/tmp/fritz.sock",
					GatewaySession:         "telegram:dm:7",
					ListenAddr:             ":9000",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "unknown command",
			args:    []string{"nope"},
			wantErr: `unknown command "nope"`,
		},
		{
			name:    "missing prompt",
			args:    []string{"run"},
			wantErr: "missing prompt",
		},
		{
			name:    "reject run chat",
			args:    []string{"run", "chat"},
			wantErr: `use "fritz chat" for interactive mode`,
		},
		{
			name:    "missing session path",
			args:    []string{"--session"},
			wantErr: "missing session path",
		},
		{
			name:    "missing fork path",
			args:    []string{"--fork"},
			wantErr: "missing fork path",
		},
		{
			name:    "missing config path",
			args:    []string{"--config"},
			wantErr: "missing config path",
		},
		{
			name:    "missing provider",
			args:    []string{"--provider"},
			wantErr: "missing provider",
		},
		{
			name:    "invalid heartbeat bool",
			args:    []string{"--heartbeat=maybe", "doctor"},
			wantErr: `invalid bool for --heartbeat: "maybe"`,
		},
		{
			name:    "invalid heartbeat interval",
			args:    []string{"--heartbeat-interval", "later", "doctor"},
			wantErr: `invalid duration for --heartbeat-interval: "later"`,
		},
		{
			name:    "missing log file",
			args:    []string{"--log-file"},
			wantErr: "missing log file",
		},
		{
			name:    "missing log level",
			args:    []string{"--log-level"},
			wantErr: "missing log level",
		},
		{
			name:    "missing telegram allow user",
			args:    []string{"--telegram-allow-user"},
			wantErr: "missing telegram allow user",
		},
		{
			name:    "missing telegram pairing token",
			args:    []string{"--telegram-pairing-token"},
			wantErr: "missing telegram pairing token",
		},
		{
			name:    "missing telegram training database path",
			args:    []string{"--telegram-training-db"},
			wantErr: "missing telegram training database path",
		},
		{
			name:    "invalid telegram poll timeout",
			args:    []string{"--telegram-poll-timeout", "later", "doctor"},
			wantErr: `invalid duration for --telegram-poll-timeout: "later"`,
		},
		{
			name:    "invalid chat help",
			args:    []string{"--chat-help=maybe", "doctor"},
			wantErr: `invalid bool for --chat-help: "maybe"`,
		},
		{
			name:    "invalid compact threshold",
			args:    []string{"--compact-threshold", "nope", "doctor"},
			wantErr: `invalid int for --compact-threshold: "nope"`,
		},
		{
			name:    "invalid command timeout",
			args:    []string{"--command-timeout", "later", "doctor"},
			wantErr: `invalid duration for --command-timeout: "later"`,
		},
		{
			name:    "missing skill path",
			args:    []string{"--skill"},
			wantErr: "missing skill path",
		},
		{
			name:    "telegram bad arg",
			args:    []string{"telegram", "--bad"},
			wantErr: `unknown telegram arg "--bad"`,
		},
		{
			name:    "missing auth subcommand",
			args:    []string{"auth"},
			wantErr: `missing auth subcommand; use "fritz auth login <provider>", "fritz auth logout <provider>", or "fritz auth status [provider]"`,
		},
		{
			name:    "missing auth provider",
			args:    []string{"auth", "login"},
			wantErr: "missing auth provider",
		},
		{
			name:    "unknown auth subcommand",
			args:    []string{"auth", "nope"},
			wantErr: `unknown auth subcommand "nope"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.args)
			if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("Parse() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func intPtr(v int) *int { return &v }

func durationPtr(v time.Duration) *time.Duration { return &v }

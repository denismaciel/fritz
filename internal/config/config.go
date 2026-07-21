package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fritz/internal/provider"
)

const (
	DefaultProvider               = provider.Gemini
	DefaultGeminiModelID          = "gemini-3-flash-preview"
	DefaultGeminiEmbeddingModelID = "gemini-embedding-001"
	DefaultOpenAICodexModelID     = "gpt-5.4-high-reasoning"
	DefaultGeminiEndpoint         = "https://generativelanguage.googleapis.com"
	DefaultOpenAICodexEndpoint    = "https://chatgpt.com/backend-api"
	DefaultOpenAICodexAuthBaseURL = "https://auth.openai.com"
	DefaultOpenAICodexClientID    = "app_EMoamEEZ73f0CkXaXp7hrann"
	DefaultOpenAICodexOriginator  = "fritz"
	DefaultOpenAICodexRedirectURL = "http://localhost:1455/auth/callback"
	DefaultTelegramEndpoint       = "https://api.telegram.org"
	DefaultSlackAPIEndpoint       = "https://slack.com/api"
	DefaultConfigPath             = ".fritz/config.json"
	DefaultSessionDir             = ""
	DefaultCompactThresholdTurns  = 20
	DefaultCompactKeepTurns       = 8
	DefaultCompactThresholdTokens = 12000
	DefaultCompactTargetTokens    = 6000
	DefaultCommandTimeout         = 30 * time.Second
	DefaultTelegramPollTimeout    = 20 * time.Second
	DefaultLogFile                = ""
	DefaultLogLevel               = "info"
)

type ChatConfig struct {
	ShowHelpOnStart bool
}

type SessionConfig struct {
	Enabled                bool
	Dir                    string
	AutoCompact            bool
	CompactThresholdTurns  int
	CompactKeepTurns       int
	CompactThresholdTokens int
	CompactTargetTokens    int
}

type PromptConfig struct {
	NoSkills   bool
	SkillPaths []string
}

type TelegramConfig struct {
	BotToken       string
	Endpoint       string
	PollTimeout    time.Duration
	PairingToken   string
	AllowedUsers   []string
	TrainingDBPath string
}

type SlackConfig struct {
	BotToken        string
	AppToken        string
	Endpoint        string
	AllowedUsers    []string
	AllowedChannels []string
	Assistant       bool
}

type HeartbeatConfig struct {
	Enabled  bool
	Interval time.Duration
}

type LogConfig struct {
	File  string
	Level string
}

type Runtime struct {
	Provider                 provider.Kind
	GeminiAPIKey             string
	ModelID                  string
	GeminiEndpoint           string
	GeminiEmbeddingModelID   string
	GeminiEmbeddingDimension int
	OpenAICodexEndpoint      string
	OpenAICodexAuthBaseURL   string
	OpenAICodexClientID      string
	OpenAICodexOriginator    string
	OpenAICodexRedirectURL   string
	Telegram                 TelegramConfig
	Slack                    SlackConfig
	Heartbeat                HeartbeatConfig
	Log                      LogConfig
	Chat                     ChatConfig
	Session                  SessionConfig
	Prompt                   PromptConfig
	CommandTimeout           time.Duration
}

type ChatConfigSource struct {
	ShowHelpOnStart *bool
}

type SessionConfigSource struct {
	Enabled                *bool
	Dir                    string
	AutoCompact            *bool
	CompactThresholdTurns  *int
	CompactKeepTurns       *int
	CompactThresholdTokens *int
	CompactTargetTokens    *int
}

type RuntimeConfigSource struct {
	CommandTimeout *time.Duration
}

type PromptConfigSource struct {
	NoSkills   *bool
	SkillPaths []string
}

type TelegramConfigSource struct {
	BotToken       string
	Endpoint       string
	PollTimeout    *time.Duration
	PairingToken   string
	AllowedUsers   []string
	TrainingDBPath string
}

type SlackConfigSource struct {
	BotToken        string
	AppToken        string
	Endpoint        string
	AllowedUsers    []string
	AllowedChannels []string
	Assistant       *bool
}

type HeartbeatConfigSource struct {
	Enabled  *bool
	Interval *time.Duration
}

type LogConfigSource struct {
	File  string
	Level string
}

type Source struct {
	Provider                 string
	GeminiAPIKey             string
	ModelID                  string
	GeminiEndpoint           string
	GeminiEmbeddingModelID   string
	GeminiEmbeddingDimension *int
	OpenAICodexEndpoint      string
	OpenAICodexAuthBaseURL   string
	OpenAICodexClientID      string
	OpenAICodexOriginator    string
	OpenAICodexRedirectURL   string
	Telegram                 TelegramConfigSource
	Slack                    SlackConfigSource
	Heartbeat                HeartbeatConfigSource
	Log                      LogConfigSource
	Chat                     ChatConfigSource
	Session                  SessionConfigSource
	Prompt                   PromptConfigSource
	Runtime                  RuntimeConfigSource
}

type Sources struct {
	Defaults   Source
	GlobalFile Source
	File       Source
	Env        Source
	Flags      Source
}

func GlobalConfigPath() string {
	if path := os.Getenv("XDG_CONFIG_HOME"); path != "" {
		return filepath.Join(path, "fritz", "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "fritz", "config.json")
}

func DefaultModelIDForProvider(kind provider.Kind) string {
	switch kind {
	case provider.OpenAICodex:
		return DefaultOpenAICodexModelID
	case provider.Gemini:
		fallthrough
	default:
		return DefaultGeminiModelID
	}
}

func DefaultSource() Source {
	return Source{
		Provider:                 string(DefaultProvider),
		ModelID:                  DefaultModelIDForProvider(DefaultProvider),
		GeminiEndpoint:           DefaultGeminiEndpoint,
		GeminiEmbeddingModelID:   DefaultGeminiEmbeddingModelID,
		GeminiEmbeddingDimension: intPtr(0),
		OpenAICodexEndpoint:      DefaultOpenAICodexEndpoint,
		OpenAICodexAuthBaseURL:   DefaultOpenAICodexAuthBaseURL,
		OpenAICodexClientID:      DefaultOpenAICodexClientID,
		OpenAICodexOriginator:    DefaultOpenAICodexOriginator,
		OpenAICodexRedirectURL:   DefaultOpenAICodexRedirectURL,
		Telegram: TelegramConfigSource{
			Endpoint:     DefaultTelegramEndpoint,
			PollTimeout:  durationPtr(DefaultTelegramPollTimeout),
			AllowedUsers: nil,
		},
		Slack: SlackConfigSource{
			Endpoint:  DefaultSlackAPIEndpoint,
			Assistant: boolPtr(true),
		},
		Heartbeat: HeartbeatConfigSource{
			Enabled:  boolPtr(false),
			Interval: durationPtr(time.Minute),
		},
		Log: LogConfigSource{
			File:  DefaultLogFile,
			Level: DefaultLogLevel,
		},
		Chat: ChatConfigSource{
			ShowHelpOnStart: boolPtr(true),
		},
		Session: SessionConfigSource{
			Enabled:                boolPtr(true),
			Dir:                    DefaultSessionDir,
			AutoCompact:            boolPtr(true),
			CompactThresholdTurns:  intPtr(DefaultCompactThresholdTurns),
			CompactKeepTurns:       intPtr(DefaultCompactKeepTurns),
			CompactThresholdTokens: intPtr(DefaultCompactThresholdTokens),
			CompactTargetTokens:    intPtr(DefaultCompactTargetTokens),
		},
		Runtime: RuntimeConfigSource{
			CommandTimeout: durationPtr(DefaultCommandTimeout),
		},
	}
}

func LoadEnv() Source {
	env := Source{
		Provider:               strings.TrimSpace(os.Getenv("FRITZ_PROVIDER")),
		GeminiAPIKey:           strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		ModelID:                strings.TrimSpace(os.Getenv("FRITZ_MODEL")),
		GeminiEndpoint:         strings.TrimSpace(os.Getenv("FRITZ_GEMINI_ENDPOINT")),
		GeminiEmbeddingModelID: strings.TrimSpace(os.Getenv("FRITZ_GEMINI_EMBEDDING_MODEL")),
		OpenAICodexEndpoint:    strings.TrimSpace(os.Getenv("FRITZ_OPENAI_CODEX_ENDPOINT")),
		OpenAICodexAuthBaseURL: strings.TrimSpace(os.Getenv("FRITZ_OPENAI_CODEX_AUTH_BASE_URL")),
		OpenAICodexClientID:    strings.TrimSpace(os.Getenv("FRITZ_OPENAI_CODEX_CLIENT_ID")),
		OpenAICodexOriginator:  strings.TrimSpace(os.Getenv("FRITZ_OPENAI_CODEX_ORIGINATOR")),
		OpenAICodexRedirectURL: strings.TrimSpace(os.Getenv("FRITZ_OPENAI_CODEX_REDIRECT_URL")),
		Telegram: TelegramConfigSource{
			BotToken:       strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
			Endpoint:       strings.TrimSpace(os.Getenv("FRITZ_TELEGRAM_ENDPOINT")),
			PairingToken:   strings.TrimSpace(os.Getenv("FRITZ_TELEGRAM_PAIRING_TOKEN")),
			TrainingDBPath: strings.TrimSpace(os.Getenv("FRITZ_TRAINING_DB")),
		},
		Slack: SlackConfigSource{
			BotToken: strings.TrimSpace(os.Getenv("SLACK_BOT_TOKEN")),
			AppToken: strings.TrimSpace(os.Getenv("SLACK_APP_TOKEN")),
			Endpoint: strings.TrimSpace(os.Getenv("FRITZ_SLACK_ENDPOINT")),
		},
		Log: LogConfigSource{
			File:  strings.TrimSpace(os.Getenv("FRITZ_LOG_FILE")),
			Level: strings.TrimSpace(os.Getenv("FRITZ_LOG_LEVEL")),
		},
	}
	if value, ok := loadBoolEnv("FRITZ_CHAT_HELP"); ok {
		env.Chat.ShowHelpOnStart = &value
	}
	if value, ok := loadBoolEnv("FRITZ_SESSION_ENABLED"); ok {
		env.Session.Enabled = &value
	}
	env.Session.Dir = strings.TrimSpace(os.Getenv("FRITZ_SESSION_DIR"))
	if value, ok := loadBoolEnv("FRITZ_AUTO_COMPACT"); ok {
		env.Session.AutoCompact = &value
	}
	if value, ok := loadIntEnv("FRITZ_COMPACT_THRESHOLD"); ok {
		env.Session.CompactThresholdTurns = &value
	}
	if value, ok := loadIntEnv("FRITZ_COMPACT_KEEP"); ok {
		env.Session.CompactKeepTurns = &value
	}
	if value, ok := loadIntEnv("FRITZ_COMPACT_THRESHOLD_TOKENS"); ok {
		env.Session.CompactThresholdTokens = &value
	}
	if value, ok := loadIntEnv("FRITZ_COMPACT_TARGET_TOKENS"); ok {
		env.Session.CompactTargetTokens = &value
	}
	if value, ok := loadDurationEnv("FRITZ_COMMAND_TIMEOUT"); ok {
		env.Runtime.CommandTimeout = &value
	}
	if value, ok := loadIntEnv("FRITZ_GEMINI_EMBEDDING_DIMENSION"); ok {
		env.GeminiEmbeddingDimension = &value
	}
	if value, ok := loadBoolEnv("FRITZ_NO_SKILLS"); ok {
		env.Prompt.NoSkills = &value
	}
	env.Prompt.SkillPaths = loadStringListEnv("FRITZ_SKILLS")
	if value, ok := loadDurationEnv("FRITZ_TELEGRAM_POLL_TIMEOUT"); ok {
		env.Telegram.PollTimeout = &value
	}
	env.Telegram.AllowedUsers = loadStringListEnv("FRITZ_TELEGRAM_ALLOWED_USERS")
	env.Slack.AllowedUsers = loadStringListEnv("FRITZ_SLACK_ALLOWED_USERS")
	env.Slack.AllowedChannels = loadStringListEnv("FRITZ_SLACK_ALLOWED_CHANNELS")
	if value, ok := loadBoolEnv("FRITZ_SLACK_ASSISTANT_ENABLED"); ok {
		env.Slack.Assistant = &value
	}
	if value, ok := loadBoolEnv("FRITZ_HEARTBEAT_ENABLED"); ok {
		env.Heartbeat.Enabled = &value
	}
	if value, ok := loadDurationEnv("FRITZ_HEARTBEAT_INTERVAL"); ok {
		env.Heartbeat.Interval = &value
	}
	return env
}

func LoadForDir(dir string, overridePath string) (Source, string, error) {
	path := strings.TrimSpace(overridePath)
	required := path != ""
	if path == "" {
		path = filepath.Join(dir, DefaultConfigPath)
	}
	source, err := LoadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !required {
			return Source{}, "", nil
		}
		return Source{}, "", err
	}
	return source, path, nil
}

func LoadFile(path string) (Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Source{}, err
	}
	var raw fileConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Source{}, err
	}
	return raw.toSource()
}

func Resolve(s Sources) Runtime {
	providerValue := firstNonEmpty(s.Flags.Provider, s.Env.Provider, s.File.Provider, s.GlobalFile.Provider, s.Defaults.Provider)
	providerKind, err := provider.Parse(providerValue)
	if err != nil {
		providerKind = provider.Kind(strings.TrimSpace(providerValue))
	}
	modelID := firstNonEmpty(
		s.Flags.ModelID,
		s.Env.ModelID,
		s.File.ModelID,
		s.GlobalFile.ModelID,
	)
	if modelID == "" {
		modelID = DefaultModelIDForProvider(providerKind)
	}
	return Runtime{
		Provider: providerKind,
		GeminiAPIKey: firstNonEmpty(
			s.Flags.GeminiAPIKey,
			s.Env.GeminiAPIKey,
			s.File.GeminiAPIKey,
			s.GlobalFile.GeminiAPIKey,
			s.Defaults.GeminiAPIKey,
		),
		ModelID: modelID,
		GeminiEndpoint: firstNonEmpty(
			s.Flags.GeminiEndpoint,
			s.Env.GeminiEndpoint,
			s.File.GeminiEndpoint,
			s.GlobalFile.GeminiEndpoint,
			s.Defaults.GeminiEndpoint,
		),
		GeminiEmbeddingModelID: firstNonEmpty(
			s.Flags.GeminiEmbeddingModelID,
			s.Env.GeminiEmbeddingModelID,
			s.File.GeminiEmbeddingModelID,
			s.GlobalFile.GeminiEmbeddingModelID,
			s.Defaults.GeminiEmbeddingModelID,
		),
		GeminiEmbeddingDimension: firstNonNilInt(
			s.Flags.GeminiEmbeddingDimension,
			s.Env.GeminiEmbeddingDimension,
			s.File.GeminiEmbeddingDimension,
			s.GlobalFile.GeminiEmbeddingDimension,
			s.Defaults.GeminiEmbeddingDimension,
		),
		OpenAICodexEndpoint: firstNonEmpty(
			s.Flags.OpenAICodexEndpoint,
			s.Env.OpenAICodexEndpoint,
			s.File.OpenAICodexEndpoint,
			s.GlobalFile.OpenAICodexEndpoint,
			s.Defaults.OpenAICodexEndpoint,
		),
		OpenAICodexAuthBaseURL: firstNonEmpty(
			s.Flags.OpenAICodexAuthBaseURL,
			s.Env.OpenAICodexAuthBaseURL,
			s.File.OpenAICodexAuthBaseURL,
			s.GlobalFile.OpenAICodexAuthBaseURL,
			s.Defaults.OpenAICodexAuthBaseURL,
		),
		OpenAICodexClientID: firstNonEmpty(
			s.Flags.OpenAICodexClientID,
			s.Env.OpenAICodexClientID,
			s.File.OpenAICodexClientID,
			s.GlobalFile.OpenAICodexClientID,
			s.Defaults.OpenAICodexClientID,
		),
		OpenAICodexOriginator: firstNonEmpty(
			s.Flags.OpenAICodexOriginator,
			s.Env.OpenAICodexOriginator,
			s.File.OpenAICodexOriginator,
			s.GlobalFile.OpenAICodexOriginator,
			s.Defaults.OpenAICodexOriginator,
		),
		OpenAICodexRedirectURL: firstNonEmpty(
			s.Flags.OpenAICodexRedirectURL,
			s.Env.OpenAICodexRedirectURL,
			s.File.OpenAICodexRedirectURL,
			s.GlobalFile.OpenAICodexRedirectURL,
			s.Defaults.OpenAICodexRedirectURL,
		),
		Telegram: TelegramConfig{
			BotToken: firstNonEmpty(
				s.Flags.Telegram.BotToken,
				s.Env.Telegram.BotToken,
				s.File.Telegram.BotToken,
				s.GlobalFile.Telegram.BotToken,
				s.Defaults.Telegram.BotToken,
			),
			Endpoint: firstNonEmpty(
				s.Flags.Telegram.Endpoint,
				s.Env.Telegram.Endpoint,
				s.File.Telegram.Endpoint,
				s.GlobalFile.Telegram.Endpoint,
				s.Defaults.Telegram.Endpoint,
			),
			PollTimeout: firstNonNilDuration(
				s.Flags.Telegram.PollTimeout,
				s.Env.Telegram.PollTimeout,
				s.File.Telegram.PollTimeout,
				s.GlobalFile.Telegram.PollTimeout,
				s.Defaults.Telegram.PollTimeout,
			),
			PairingToken: firstNonEmpty(
				s.Flags.Telegram.PairingToken,
				s.Env.Telegram.PairingToken,
				s.File.Telegram.PairingToken,
				s.GlobalFile.Telegram.PairingToken,
				s.Defaults.Telegram.PairingToken,
			),
			AllowedUsers: mergeStringLists(
				s.GlobalFile.Telegram.AllowedUsers,
				s.File.Telegram.AllowedUsers,
				s.Env.Telegram.AllowedUsers,
				s.Flags.Telegram.AllowedUsers,
			),
			TrainingDBPath: firstNonEmpty(
				s.Flags.Telegram.TrainingDBPath,
				s.Env.Telegram.TrainingDBPath,
				s.File.Telegram.TrainingDBPath,
				s.GlobalFile.Telegram.TrainingDBPath,
				s.Defaults.Telegram.TrainingDBPath,
			),
		},
		Slack: SlackConfig{
			BotToken: firstNonEmpty(
				s.Flags.Slack.BotToken,
				s.Env.Slack.BotToken,
				s.File.Slack.BotToken,
				s.GlobalFile.Slack.BotToken,
				s.Defaults.Slack.BotToken,
			),
			AppToken: firstNonEmpty(
				s.Flags.Slack.AppToken,
				s.Env.Slack.AppToken,
				s.File.Slack.AppToken,
				s.GlobalFile.Slack.AppToken,
				s.Defaults.Slack.AppToken,
			),
			Endpoint: firstNonEmpty(
				s.Flags.Slack.Endpoint,
				s.Env.Slack.Endpoint,
				s.File.Slack.Endpoint,
				s.GlobalFile.Slack.Endpoint,
				s.Defaults.Slack.Endpoint,
			),
			AllowedUsers: mergeStringLists(
				s.GlobalFile.Slack.AllowedUsers,
				s.File.Slack.AllowedUsers,
				s.Env.Slack.AllowedUsers,
				s.Flags.Slack.AllowedUsers,
			),
			AllowedChannels: mergeStringLists(
				s.GlobalFile.Slack.AllowedChannels,
				s.File.Slack.AllowedChannels,
				s.Env.Slack.AllowedChannels,
				s.Flags.Slack.AllowedChannels,
			),
			Assistant: firstNonNilBool(
				s.Flags.Slack.Assistant,
				s.Env.Slack.Assistant,
				s.File.Slack.Assistant,
				s.GlobalFile.Slack.Assistant,
				s.Defaults.Slack.Assistant,
			),
		},
		Heartbeat: HeartbeatConfig{
			Enabled: firstNonNilBool(
				s.Flags.Heartbeat.Enabled,
				s.Env.Heartbeat.Enabled,
				s.File.Heartbeat.Enabled,
				s.GlobalFile.Heartbeat.Enabled,
				s.Defaults.Heartbeat.Enabled,
			),
			Interval: firstNonNilDuration(
				s.Flags.Heartbeat.Interval,
				s.Env.Heartbeat.Interval,
				s.File.Heartbeat.Interval,
				s.Defaults.Heartbeat.Interval,
			),
		},
		Log: LogConfig{
			File: firstNonEmpty(
				s.Flags.Log.File,
				s.Env.Log.File,
				s.File.Log.File,
				s.GlobalFile.Log.File,
				s.Defaults.Log.File,
			),
			Level: firstNonEmpty(
				s.Flags.Log.Level,
				s.Env.Log.Level,
				s.File.Log.Level,
				s.GlobalFile.Log.Level,
				s.Defaults.Log.Level,
			),
		},
		Chat: ChatConfig{
			ShowHelpOnStart: firstNonNilBool(
				s.Flags.Chat.ShowHelpOnStart,
				s.Env.Chat.ShowHelpOnStart,
				s.File.Chat.ShowHelpOnStart,
				s.GlobalFile.Chat.ShowHelpOnStart,
				s.Defaults.Chat.ShowHelpOnStart,
			),
		},
		Session: SessionConfig{
			Enabled: firstNonNilBool(
				s.Flags.Session.Enabled,
				s.Env.Session.Enabled,
				s.File.Session.Enabled,
				s.GlobalFile.Session.Enabled,
				s.Defaults.Session.Enabled,
			),
			Dir: firstNonEmpty(
				s.Flags.Session.Dir,
				s.Env.Session.Dir,
				s.File.Session.Dir,
				s.GlobalFile.Session.Dir,
				s.Defaults.Session.Dir,
			),
			AutoCompact: firstNonNilBool(
				s.Flags.Session.AutoCompact,
				s.Env.Session.AutoCompact,
				s.File.Session.AutoCompact,
				s.GlobalFile.Session.AutoCompact,
				s.Defaults.Session.AutoCompact,
			),
			CompactThresholdTurns: firstNonNilInt(
				s.Flags.Session.CompactThresholdTurns,
				s.Env.Session.CompactThresholdTurns,
				s.File.Session.CompactThresholdTurns,
				s.GlobalFile.Session.CompactThresholdTurns,
				s.Defaults.Session.CompactThresholdTurns,
			),
			CompactKeepTurns: firstNonNilInt(
				s.Flags.Session.CompactKeepTurns,
				s.Env.Session.CompactKeepTurns,
				s.File.Session.CompactKeepTurns,
				s.GlobalFile.Session.CompactKeepTurns,
				s.Defaults.Session.CompactKeepTurns,
			),
			CompactThresholdTokens: firstNonNilInt(
				s.Flags.Session.CompactThresholdTokens,
				s.Env.Session.CompactThresholdTokens,
				s.File.Session.CompactThresholdTokens,
				s.GlobalFile.Session.CompactThresholdTokens,
				s.Defaults.Session.CompactThresholdTokens,
			),
			CompactTargetTokens: firstNonNilInt(
				s.Flags.Session.CompactTargetTokens,
				s.Env.Session.CompactTargetTokens,
				s.File.Session.CompactTargetTokens,
				s.Defaults.Session.CompactTargetTokens,
			),
		},
		Prompt: PromptConfig{
			NoSkills: firstNonNilBool(
				s.Flags.Prompt.NoSkills,
				s.Env.Prompt.NoSkills,
				s.File.Prompt.NoSkills,
				s.GlobalFile.Prompt.NoSkills,
				s.Defaults.Prompt.NoSkills,
			),
			SkillPaths: mergeStringLists(
				s.GlobalFile.Prompt.SkillPaths,
				s.File.Prompt.SkillPaths,
				s.Env.Prompt.SkillPaths,
				s.Flags.Prompt.SkillPaths,
			),
		},
		CommandTimeout: firstNonNilDuration(
			s.Flags.Runtime.CommandTimeout,
			s.Env.Runtime.CommandTimeout,
			s.File.Runtime.CommandTimeout,
			s.GlobalFile.Runtime.CommandTimeout,
			s.Defaults.Runtime.CommandTimeout,
		),
	}
}

func (c Runtime) HasGeminiAPIKey() bool {
	return c.GeminiAPIKey != ""
}

func (c Runtime) Validate() error {
	if _, err := provider.Parse(string(c.Provider)); err != nil {
		return err
	}
	if err := validateModelForProvider(c.Provider, c.ModelID); err != nil {
		return err
	}
	switch c.Provider {
	case provider.Gemini:
		if !c.HasGeminiAPIKey() {
			return errors.New("missing GEMINI_API_KEY")
		}
	case provider.OpenAICodex:
		return nil
	}
	return nil
}

func validateModelForProvider(kind provider.Kind, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	lower := strings.ToLower(modelID)
	switch kind {
	case provider.Gemini:
		if strings.HasPrefix(lower, "gpt-") {
			return fmt.Errorf("model %q is not valid for provider %q", modelID, kind)
		}
	case provider.OpenAICodex:
		if strings.HasPrefix(lower, "gemini-") {
			return fmt.Errorf("model %q is not valid for provider %q", modelID, kind)
		}
	}
	return nil
}

func (c Runtime) ValidateTelegram() error {
	if strings.TrimSpace(c.Telegram.BotToken) == "" {
		return errors.New("missing TELEGRAM_BOT_TOKEN")
	}
	return nil
}

func (c Runtime) ValidateSlack() error {
	if strings.TrimSpace(c.Slack.BotToken) == "" {
		return errors.New("missing SLACK_BOT_TOKEN")
	}
	if strings.TrimSpace(c.Slack.AppToken) == "" {
		return errors.New("missing SLACK_APP_TOKEN")
	}
	return nil
}

func boolPtr(v bool) *bool                       { return &v }
func intPtr(v int) *int                          { return &v }
func durationPtr(v time.Duration) *time.Duration { return &v }

func loadBoolEnv(name string) (bool, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	switch value {
	case "1", "true", "TRUE", "yes":
		return true, true
	case "0", "false", "FALSE", "no":
		return false, true
	default:
		return false, false
	}
}

func loadIntEnv(name string) (int, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func loadDurationEnv(name string) (time.Duration, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, false
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func loadStringListEnv(name string) []string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == os.PathListSeparator || r == ','
	})
	return compactStrings(fields)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNilBool(values ...*bool) bool {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return false
}

func firstNonNilInt(values ...*int) int {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}

func firstNonNilDuration(values ...*time.Duration) time.Duration {
	for _, value := range values {
		if value != nil {
			return *value
		}
	}
	return 0
}

func mergeStringLists(values ...[]string) []string {
	var out []string
	for _, list := range values {
		out = append(out, compactStrings(list)...)
	}
	return out
}

func compactStrings(values []string) []string {
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

type fileConfig struct {
	Provider                 string              `json:"provider"`
	GeminiAPIKey             string              `json:"geminiApiKey"`
	ModelID                  string              `json:"model"`
	GeminiEndpoint           string              `json:"geminiEndpoint"`
	GeminiEmbeddingModelID   string              `json:"geminiEmbeddingModel"`
	GeminiEmbeddingDimension *int                `json:"geminiEmbeddingDimension"`
	OpenAICodexEndpoint      string              `json:"openAICodexEndpoint"`
	OpenAICodexAuthBaseURL   string              `json:"openAICodexAuthBaseURL"`
	OpenAICodexClientID      string              `json:"openAICodexClientID"`
	OpenAICodexOriginator    string              `json:"openAICodexOriginator"`
	OpenAICodexRedirectURL   string              `json:"openAICodexRedirectURL"`
	Telegram                 fileTelegramConfig  `json:"telegram"`
	Slack                    fileSlackConfig     `json:"slack"`
	Heartbeat                fileHeartbeatConfig `json:"heartbeat"`
	Log                      fileLogConfig       `json:"log"`
	Chat                     fileChatConfig      `json:"chat"`
	Session                  fileSessionConfig   `json:"session"`
	Prompt                   filePromptConfig    `json:"prompt"`
	Runtime                  fileRuntimeConfig   `json:"runtime"`
}

type fileTelegramConfig struct {
	BotToken       string   `json:"botToken"`
	Endpoint       string   `json:"endpoint"`
	PollTimeout    string   `json:"pollTimeout"`
	PairingToken   string   `json:"pairingToken"`
	AllowedUsers   []string `json:"allowedUsers"`
	TrainingDBPath string   `json:"trainingDbPath"`
}

type fileSlackConfig struct {
	BotToken        string   `json:"botToken"`
	AppToken        string   `json:"appToken"`
	Endpoint        string   `json:"endpoint"`
	AllowedUsers    []string `json:"allowedUsers"`
	AllowedChannels []string `json:"allowedChannels"`
	Assistant       *bool    `json:"assistant"`
}

type fileHeartbeatConfig struct {
	Enabled  *bool  `json:"enabled"`
	Interval string `json:"interval"`
}

type fileLogConfig struct {
	File  string `json:"file"`
	Level string `json:"level"`
}

type fileChatConfig struct {
	ShowHelpOnStart *bool `json:"showHelpOnStart"`
}

type fileSessionConfig struct {
	Enabled                *bool  `json:"enabled"`
	Dir                    string `json:"dir"`
	AutoCompact            *bool  `json:"autoCompact"`
	CompactThresholdTurns  *int   `json:"compactThresholdTurns"`
	CompactKeepTurns       *int   `json:"compactKeepTurns"`
	CompactThresholdTokens *int   `json:"compactThresholdTokens"`
	CompactTargetTokens    *int   `json:"compactTargetTokens"`
}

type fileRuntimeConfig struct {
	CommandTimeout string `json:"commandTimeout"`
}

type filePromptConfig struct {
	NoSkills   *bool    `json:"noSkills"`
	SkillPaths []string `json:"skillPaths"`
}

func (f fileConfig) toSource() (Source, error) {
	source := Source{
		Provider:                 strings.TrimSpace(f.Provider),
		GeminiAPIKey:             strings.TrimSpace(f.GeminiAPIKey),
		ModelID:                  strings.TrimSpace(f.ModelID),
		GeminiEndpoint:           strings.TrimSpace(f.GeminiEndpoint),
		GeminiEmbeddingModelID:   strings.TrimSpace(f.GeminiEmbeddingModelID),
		GeminiEmbeddingDimension: f.GeminiEmbeddingDimension,
		OpenAICodexEndpoint:      strings.TrimSpace(f.OpenAICodexEndpoint),
		OpenAICodexAuthBaseURL:   strings.TrimSpace(f.OpenAICodexAuthBaseURL),
		OpenAICodexClientID:      strings.TrimSpace(f.OpenAICodexClientID),
		OpenAICodexOriginator:    strings.TrimSpace(f.OpenAICodexOriginator),
		OpenAICodexRedirectURL:   strings.TrimSpace(f.OpenAICodexRedirectURL),
		Telegram: TelegramConfigSource{
			BotToken:       strings.TrimSpace(f.Telegram.BotToken),
			Endpoint:       strings.TrimSpace(f.Telegram.Endpoint),
			PairingToken:   strings.TrimSpace(f.Telegram.PairingToken),
			AllowedUsers:   compactStrings(f.Telegram.AllowedUsers),
			TrainingDBPath: strings.TrimSpace(f.Telegram.TrainingDBPath),
		},
		Slack: SlackConfigSource{
			BotToken:        strings.TrimSpace(f.Slack.BotToken),
			AppToken:        strings.TrimSpace(f.Slack.AppToken),
			Endpoint:        strings.TrimSpace(f.Slack.Endpoint),
			AllowedUsers:    compactStrings(f.Slack.AllowedUsers),
			AllowedChannels: compactStrings(f.Slack.AllowedChannels),
			Assistant:       f.Slack.Assistant,
		},
		Heartbeat: HeartbeatConfigSource{
			Enabled: f.Heartbeat.Enabled,
		},
		Log: LogConfigSource{
			File:  strings.TrimSpace(f.Log.File),
			Level: strings.TrimSpace(f.Log.Level),
		},
		Chat: ChatConfigSource{
			ShowHelpOnStart: f.Chat.ShowHelpOnStart,
		},
		Session: SessionConfigSource{
			Enabled:                f.Session.Enabled,
			Dir:                    strings.TrimSpace(f.Session.Dir),
			AutoCompact:            f.Session.AutoCompact,
			CompactThresholdTurns:  f.Session.CompactThresholdTurns,
			CompactKeepTurns:       f.Session.CompactKeepTurns,
			CompactThresholdTokens: f.Session.CompactThresholdTokens,
			CompactTargetTokens:    f.Session.CompactTargetTokens,
		},
		Prompt: PromptConfigSource{
			NoSkills:   f.Prompt.NoSkills,
			SkillPaths: compactStrings(f.Prompt.SkillPaths),
		},
	}
	if strings.TrimSpace(f.Runtime.CommandTimeout) != "" {
		value, err := time.ParseDuration(strings.TrimSpace(f.Runtime.CommandTimeout))
		if err != nil {
			return Source{}, err
		}
		source.Runtime.CommandTimeout = &value
	}
	if strings.TrimSpace(f.Telegram.PollTimeout) != "" {
		value, err := time.ParseDuration(strings.TrimSpace(f.Telegram.PollTimeout))
		if err != nil {
			return Source{}, err
		}
		source.Telegram.PollTimeout = &value
	}
	if strings.TrimSpace(f.Heartbeat.Interval) != "" {
		value, err := time.ParseDuration(strings.TrimSpace(f.Heartbeat.Interval))
		if err != nil {
			return Source{}, err
		}
		source.Heartbeat.Interval = &value
	}
	return source, nil
}

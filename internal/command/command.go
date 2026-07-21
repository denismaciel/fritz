package command

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"fritz/internal/brand"
)

type Command interface {
	isCommand()
}

type SessionOptions struct {
	Continue   bool
	Session    string
	Fork       string
	NoSession  bool
	NewSession bool
}

type ConfigOptions struct {
	Path                   string
	Provider               string
	ModelID                string
	GeminiEndpoint         string
	OpenAICodexEndpoint    string
	OpenAICodexAuthBaseURL string
	OpenAICodexClientID    string
	OpenAICodexOriginator  string
	OpenAICodexRedirectURL string
	TelegramBotToken       string
	TelegramEndpoint       string
	TelegramPollTimeout    *time.Duration
	TelegramPairingToken   string
	TelegramAllowedUsers   []string
	TelegramTrainingDBPath string
	SlackBotToken          string
	SlackAppToken          string
	SlackEndpoint          string
	SlackAllowedUsers      []string
	SlackAllowedChannels   []string
	SlackAssistantEnabled  *bool
	HeartbeatEnabled       *bool
	HeartbeatInterval      *time.Duration
	LogFile                string
	LogLevel               string
	ServerURL              string
	ObserveSocket          string
	GatewaySession         string
	ListenAddr             string
	SessionDir             string
	ChatHelp               *bool
	AutoCompact            *bool
	CompactThresholdTurns  *int
	CompactKeepTurns       *int
	CompactThresholdTokens *int
	CompactTargetTokens    *int
	CommandTimeout         *time.Duration
	SkillPaths             []string
	NoSkills               *bool
}

type Help struct {
	Config ConfigOptions
}

func (Help) isCommand() {}

type Doctor struct {
	Config ConfigOptions
}

func (Doctor) isCommand() {}

type Run struct {
	Prompt  string
	Session SessionOptions
	Config  ConfigOptions
}

func (Run) isCommand() {}

type Chat struct {
	Session SessionOptions
	Config  ConfigOptions
}

func (Chat) isCommand() {}

type Serve struct {
	Config ConfigOptions
}

func (Serve) isCommand() {}

type Attach struct {
	RunID  string
	Config ConfigOptions
}

func (Attach) isCommand() {}

type Telegram struct {
	PollOnce bool
	Config   ConfigOptions
}

func (Telegram) isCommand() {}

type Slack struct {
	Config ConfigOptions
}

func (Slack) isCommand() {}

type AuthLogin struct {
	Provider string
	Config   ConfigOptions
}

func (AuthLogin) isCommand() {}

type AuthLogout struct {
	Provider string
	Config   ConfigOptions
}

func (AuthLogout) isCommand() {}

type AuthStatus struct {
	Provider string
	Config   ConfigOptions
}

func (AuthStatus) isCommand() {}

func Parse(args []string) (Command, error) {
	if len(args) == 0 {
		return Chat{}, nil
	}

	session, cfg, remaining, err := parseOptions(args)
	if err != nil {
		return nil, err
	}
	if len(remaining) == 0 {
		return Chat{Session: session, Config: cfg}, nil
	}

	switch remaining[0] {
	case "help", "--help", "-h":
		return Help{Config: cfg}, nil
	case "doctor":
		return Doctor{Config: cfg}, nil
	case "chat":
		return Chat{Session: session, Config: cfg}, nil
	case "serve":
		return Serve{Config: cfg}, nil
	case "attach":
		runID := ""
		if len(remaining) > 1 {
			runID = strings.TrimSpace(remaining[1])
		}
		if len(remaining) > 2 {
			return nil, fmt.Errorf("unexpected attach arg %q", remaining[2])
		}
		return Attach{RunID: runID, Config: cfg}, nil
	case "telegram":
		pollOnce, err := parseTelegramArgs(remaining[1:])
		if err != nil {
			return nil, err
		}
		return Telegram{PollOnce: pollOnce, Config: cfg}, nil
	case "auth":
		return parseAuthArgs(cfg, remaining[1:])
	case "run":
		prompt := strings.TrimSpace(strings.Join(remaining[1:], " "))
		if prompt == "" {
			return nil, errors.New("missing prompt")
		}
		if prompt == "chat" {
			return nil, fmt.Errorf(`use "%s chat" for interactive mode`, brand.CLIName)
		}
		return Run{Prompt: prompt, Session: session, Config: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown command %q", remaining[0])
	}
}

func ParseTelegramProcess(args []string) (Telegram, error) {
	_, cfg, remaining, err := parseOptions(args)
	if err != nil {
		return Telegram{}, err
	}
	pollOnce, err := parseTelegramArgs(remaining)
	if err != nil {
		return Telegram{}, err
	}
	return Telegram{PollOnce: pollOnce, Config: cfg}, nil
}

func ParseSlackProcess(args []string) (Slack, error) {
	_, cfg, remaining, err := parseOptions(args)
	if err != nil {
		return Slack{}, err
	}
	if len(remaining) > 0 {
		return Slack{}, fmt.Errorf("unknown slack arg %q", remaining[0])
	}
	return Slack{Config: cfg}, nil
}

func parseAuthArgs(cfg ConfigOptions, args []string) (Command, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf(`missing auth subcommand; use "%s auth login <provider>", "%s auth logout <provider>", or "%s auth status [provider]"`, brand.CLIName, brand.CLIName, brand.CLIName)
	}
	switch args[0] {
	case "login":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return nil, errors.New("missing auth provider")
		}
		return AuthLogin{Provider: strings.TrimSpace(args[1]), Config: cfg}, nil
	case "logout":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return nil, errors.New("missing auth provider")
		}
		return AuthLogout{Provider: strings.TrimSpace(args[1]), Config: cfg}, nil
	case "status":
		if len(args) >= 2 {
			return AuthStatus{Provider: strings.TrimSpace(args[1]), Config: cfg}, nil
		}
		return AuthStatus{Config: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown auth subcommand %q", args[0])
	}
}

func parseOptions(args []string) (SessionOptions, ConfigOptions, []string, error) {
	var session SessionOptions
	var cfg ConfigOptions
	remaining := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--continue", "-c":
			session.Continue = true
		case "--session":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing session path")
			}
			i++
			session.Session = args[i]
		case "--fork":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing fork path")
			}
			i++
			session.Fork = args[i]
		case "--no-session":
			session.NoSession = true
		case "--new-session":
			session.NewSession = true
		case "--config":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing config path")
			}
			i++
			cfg.Path = args[i]
		case "--model":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing model id")
			}
			i++
			cfg.ModelID = args[i]
		case "--provider":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing provider")
			}
			i++
			cfg.Provider = args[i]
		case "--gemini-endpoint":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing gemini endpoint")
			}
			i++
			cfg.GeminiEndpoint = args[i]
		case "--openai-codex-endpoint":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing openai codex endpoint")
			}
			i++
			cfg.OpenAICodexEndpoint = args[i]
		case "--openai-codex-auth-base-url":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing openai codex auth base url")
			}
			i++
			cfg.OpenAICodexAuthBaseURL = args[i]
		case "--openai-codex-client-id":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing openai codex client id")
			}
			i++
			cfg.OpenAICodexClientID = args[i]
		case "--openai-codex-originator":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing openai codex originator")
			}
			i++
			cfg.OpenAICodexOriginator = args[i]
		case "--openai-codex-redirect-url":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing openai codex redirect url")
			}
			i++
			cfg.OpenAICodexRedirectURL = args[i]
		case "--telegram-bot-token":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram bot token")
			}
			i++
			cfg.TelegramBotToken = args[i]
		case "--telegram-endpoint":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram endpoint")
			}
			i++
			cfg.TelegramEndpoint = args[i]
		case "--telegram-poll-timeout":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram poll timeout")
			}
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid duration for --telegram-poll-timeout: %q", args[i])
			}
			cfg.TelegramPollTimeout = &value
		case "--telegram-pairing-token":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram pairing token")
			}
			i++
			cfg.TelegramPairingToken = args[i]
		case "--telegram-allow-user":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram allow user")
			}
			i++
			cfg.TelegramAllowedUsers = append(cfg.TelegramAllowedUsers, args[i])
		case "--telegram-training-db":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing telegram training database path")
			}
			i++
			cfg.TelegramTrainingDBPath = args[i]
		case "--slack-bot-token":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing slack bot token")
			}
			i++
			cfg.SlackBotToken = args[i]
		case "--slack-app-token":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing slack app token")
			}
			i++
			cfg.SlackAppToken = args[i]
		case "--slack-endpoint":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing slack endpoint")
			}
			i++
			cfg.SlackEndpoint = args[i]
		case "--slack-allow-user":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing slack allow user")
			}
			i++
			cfg.SlackAllowedUsers = append(cfg.SlackAllowedUsers, args[i])
		case "--slack-allow-channel":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing slack allow channel")
			}
			i++
			cfg.SlackAllowedChannels = append(cfg.SlackAllowedChannels, args[i])
		case "--heartbeat-interval":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing heartbeat interval")
			}
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid duration for --heartbeat-interval: %q", args[i])
			}
			cfg.HeartbeatInterval = &value
		case "--log-file":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing log file")
			}
			i++
			cfg.LogFile = args[i]
		case "--log-level":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing log level")
			}
			i++
			cfg.LogLevel = args[i]
		case "--server":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing server url")
			}
			i++
			cfg.ServerURL = args[i]
		case "--observe-socket":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing observe socket")
			}
			i++
			cfg.ObserveSocket = args[i]
		case "--gateway-session":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing gateway session")
			}
			i++
			cfg.GatewaySession = args[i]
		case "--listen":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing listen address")
			}
			i++
			cfg.ListenAddr = args[i]
		case "--session-dir":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing session dir")
			}
			i++
			cfg.SessionDir = args[i]
		case "--compact-threshold":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing compact threshold")
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid int for --compact-threshold: %q", args[i])
			}
			cfg.CompactThresholdTurns = &value
		case "--compact-keep":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing compact keep")
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid int for --compact-keep: %q", args[i])
			}
			cfg.CompactKeepTurns = &value
		case "--compact-threshold-tokens":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing compact threshold tokens")
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid int for --compact-threshold-tokens: %q", args[i])
			}
			cfg.CompactThresholdTokens = &value
		case "--compact-target-tokens":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing compact target tokens")
			}
			i++
			value, err := strconv.Atoi(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid int for --compact-target-tokens: %q", args[i])
			}
			cfg.CompactTargetTokens = &value
		case "--command-timeout":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing command timeout")
			}
			i++
			value, err := time.ParseDuration(args[i])
			if err != nil {
				return SessionOptions{}, ConfigOptions{}, nil, fmt.Errorf("invalid duration for --command-timeout: %q", args[i])
			}
			cfg.CommandTimeout = &value
		case "--skill":
			if i+1 >= len(args) {
				return SessionOptions{}, ConfigOptions{}, nil, errors.New("missing skill path")
			}
			i++
			cfg.SkillPaths = append(cfg.SkillPaths, args[i])
		case "--no-skills":
			value := true
			cfg.NoSkills = &value
		default:
			if value, ok, err := parseBoolFlag(args[i], "--chat-help"); ok {
				if err != nil {
					return SessionOptions{}, ConfigOptions{}, nil, err
				}
				cfg.ChatHelp = &value
				continue
			}
			if value, ok, err := parseBoolFlag(args[i], "--auto-compact"); ok {
				if err != nil {
					return SessionOptions{}, ConfigOptions{}, nil, err
				}
				cfg.AutoCompact = &value
				continue
			}
			if value, ok, err := parseBoolFlag(args[i], "--heartbeat"); ok {
				if err != nil {
					return SessionOptions{}, ConfigOptions{}, nil, err
				}
				cfg.HeartbeatEnabled = &value
				continue
			}
			if value, ok, err := parseBoolFlag(args[i], "--slack-assistant"); ok {
				if err != nil {
					return SessionOptions{}, ConfigOptions{}, nil, err
				}
				cfg.SlackAssistantEnabled = &value
				continue
			}
			remaining = append(remaining, args[i])
		}
	}
	return session, cfg, remaining, nil
}

func parseBoolFlag(arg string, name string) (bool, bool, error) {
	if !strings.HasPrefix(arg, name+"=") {
		return false, false, nil
	}
	raw := strings.TrimPrefix(arg, name+"=")
	switch raw {
	case "1", "true", "TRUE", "yes":
		return true, true, nil
	case "0", "false", "FALSE", "no":
		return false, true, nil
	default:
		return false, true, fmt.Errorf("invalid bool for %s: %q", name, raw)
	}
}

func parseTelegramArgs(args []string) (bool, error) {
	pollOnce := false
	for _, arg := range args {
		switch arg {
		case "--poll-once":
			pollOnce = true
		default:
			return false, fmt.Errorf("unknown telegram arg %q", arg)
		}
	}
	return pollOnce, nil
}

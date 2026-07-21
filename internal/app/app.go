package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fritz/internal/adapters/slack"
	"fritz/internal/adapters/telegram"
	"fritz/internal/agent"
	"fritz/internal/auth"
	"fritz/internal/authstore"
	"fritz/internal/brand"
	"fritz/internal/chat"
	"fritz/internal/command"
	"fritz/internal/config"
	"fritz/internal/engine"
	"fritz/internal/gemini"
	"fritz/internal/heartbeat"
	"fritz/internal/httpapi"
	ingressruntime "fritz/internal/ingress"
	"fritz/internal/logx"
	"fritz/internal/model"
	"fritz/internal/observe"
	"fritz/internal/openaicodex"
	"fritz/internal/prompt"
	"fritz/internal/provider"
	"fritz/internal/reminderwake"
	"fritz/internal/session"
	"fritz/internal/terminalui"
	"fritz/internal/tool"
	"fritz/internal/trainingplan"
	transcriptiongemini "fritz/internal/transcription/gemini"
	"golang.org/x/term"
)

func Run(ctx context.Context, args []string) error {
	return RunAgent(ctx, args)
}

func RunAgent(ctx context.Context, args []string) error {
	cwd := mustGetwd()
	return runWithProfile(ctx, args, os.Stdin, os.Stdout, func(cfg config.Runtime) model.Client {
		return defaultClientFactory(cwd, cfg)
	}, nil, prompt.ProfileCoding, false)
}

var newHeartbeatSource = func(config.Runtime) heartbeat.Source {
	return heartbeat.NullSource{}
}

var newAuthStore = func() authstore.Store {
	return authstore.NewGlobalFileStore()
}

var createOpenAICodexAuthorizationFlow = openaicodex.CreateAuthorizationFlow
var openBrowserURL = tryOpenBrowser
var startOpenAICodexCallbackServer = openaicodex.StartCallbackServer
var exchangeOpenAICodexAuthorizationCode = openaicodex.ExchangeAuthorizationCode

func defaultClientFactory(cwd string, cfg config.Runtime) model.Client {
	switch cfg.Provider {
	case provider.OpenAICodex:
		resolver := auth.NewResolver(newAuthStore())
		return openaicodex.NewClient(func(ctx context.Context) (provider.RequestAuth, error) {
			return resolver.Resolve(ctx, cfg)
		},
			openaicodex.WithEndpoint(cfg.OpenAICodexEndpoint),
			openaicodex.WithModel(cfg.ModelID),
		)
	default:
		return gemini.NewClient(
			cfg.GeminiAPIKey,
			gemini.WithEndpoint(cfg.GeminiEndpoint),
			gemini.WithModel(cfg.ModelID),
		)
	}
}

func RunTelegramProcess(ctx context.Context, args []string) error {
	cwd := mustGetwd()
	return runTelegramProcess(ctx, args, os.Stdout, cwd, func(cfg config.Runtime) model.Client {
		return defaultClientFactory(cwd, cfg)
	}, nil)
}

func RunSlackProcess(ctx context.Context, args []string) error {
	cwd := mustGetwd()
	return runSlackProcess(ctx, args, os.Stdout, cwd, func(cfg config.Runtime) model.Client {
		return defaultClientFactory(cwd, cfg)
	}, nil)
}

func run(
	ctx context.Context,
	args []string,
	in io.Reader,
	out io.Writer,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
) error {
	return runWithProfile(ctx, args, in, out, newClient, registry, prompt.ProfileCoding, false)
}

func runWithProfile(
	ctx context.Context,
	args []string,
	in io.Reader,
	out io.Writer,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
	profile prompt.Profile,
	allowTelegram bool,
) error {
	cmd, err := command.Parse(args)
	if err != nil {
		return wrapError("command", err)
	}

	cwd := mustGetwd()

	switch cmd := cmd.(type) {
	case command.Help:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		printAgentUsage(out)
		return nil
	case command.Doctor:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runDoctor(cwd, out, cfg)
	case command.Run:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runAgent(ctx, out, cwd, cmd, cfg, newClient, registry, profile)
	case command.Chat:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runChat(ctx, in, out, cwd, cmd, cfg, newClient, registry, profile)
	case command.Serve:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runServe(ctx, out, cwd, cmd, cfg, newClient, registry, profile)
	case command.Attach:
		return runAttach(ctx, out, cwd, cmd)
	case command.Telegram:
		if !allowTelegram {
			return wrapError("command", errors.New("use fritz-telegram"))
		}
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runTelegram(ctx, out, cwd, cmd, cfg, newClient, registry)
	case command.Slack:
		return wrapError("command", errors.New("use fritz-slack"))
	case command.AuthLogin:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runAuthLogin(ctx, in, out, cwd, cfg, cmd)
	case command.AuthLogout:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runAuthLogout(out, cwd, cfg, cmd)
	case command.AuthStatus:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runAuthStatus(out, cwd, cfg, cmd)
	default:
		return wrapError("command", fmt.Errorf("unsupported command %T", cmd))
	}
}

func defaultToolRegistry() *tool.Registry {
	return toolRegistryForProfile(config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	}), prompt.ProfileCoding)
}

func toolRegistryForProfile(cfg config.Runtime, profile prompt.Profile) *tool.Registry {
	registry := tool.NewRegistry()
	root := mustGetwd()
	registerCodingTools(registry, root, cfg)
	if profile == prompt.ProfileGateway {
		registerGatewayTools(registry, root)
	}
	return registry
}

func registerCodingTools(registry *tool.Registry, root string, cfg config.Runtime) {
	registry.Register(tool.NewBashTool(root, tool.WithDefaultTimeout(cfg.CommandTimeout)))
	registry.Register(tool.NewEditTool(root, 128*1024))
	registry.Register(tool.NewFindTool(root))
	registry.Register(tool.NewGrepTool(root))
	registry.Register(tool.NewLsTool(root))
	registry.Register(tool.NewReadTool(root, 128*1024))
	registry.Register(tool.NewWebSearchTool(cfg.GeminiAPIKey, cfg.GeminiEndpoint))
	registry.Register(tool.NewWriteTool(root))
}

func registerGatewayTools(registry *tool.Registry, root string) {
	registry.Register(tool.NewReminderDeleteTool(root))
	registry.Register(tool.NewReminderListTool(root))
	registry.Register(tool.NewReminderSetTool(root))
	registry.Register(tool.NewSecretDeleteTool(root))
	registry.Register(tool.NewSecretListTool(root))
	registry.Register(tool.NewSecretSetTool(root))
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func resolveRuntimeConfig(cwd string, flags command.ConfigOptions) (config.Runtime, error) {
	globalFileSource, err := config.LoadFile(config.GlobalConfigPath())
	if err != nil && !os.IsNotExist(err) {
		return config.Runtime{}, err
	}
	fileSource, _, err := config.LoadForDir(cwd, flags.Path)
	if err != nil {
		return config.Runtime{}, err
	}
	return config.Resolve(config.Sources{
		Defaults:   config.DefaultSource(),
		GlobalFile: globalFileSource,
		File:       fileSource,
		Env:        config.LoadEnv(),
		Flags:      configSourceFromCommand(flags),
	}), nil
}

func runDoctor(cwd string, out io.Writer, cfg config.Runtime) error {
	sessionDir, err := config.ResolveSessionDir(cwd, cfg.Session.Dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "provider: %s\n", cfg.Provider)
	fmt.Fprintf(out, "endpoint: %s\n", providerEndpoint(cfg))
	fmt.Fprintf(out, "log_file: %s\n", config.ResolveLogFile(cwd, cfg.Log.File))
	fmt.Fprintf(out, "log_level: %s\n", cfg.Log.Level)
	switch cfg.Provider {
	case provider.Gemini:
		if cfg.HasGeminiAPIKey() {
			fmt.Fprintln(out, "GEMINI_API_KEY: set")
		} else {
			fmt.Fprintln(out, "GEMINI_API_KEY: missing")
		}
	case provider.OpenAICodex:
		fmt.Fprintf(out, "auth_base_url: %s\n", cfg.OpenAICodexAuthBaseURL)
		fmt.Fprintf(out, "oauth_client_id: %s\n", cfg.OpenAICodexClientID)
		fmt.Fprintf(out, "oauth_redirect_url: %s\n", cfg.OpenAICodexRedirectURL)
		status := "missing"
		if entry, ok, err := newAuthStore().Get(provider.OpenAICodex); err == nil && ok {
			status = authstore.FormatStatus(entry)
		}
		fmt.Fprintf(out, "openai_codex_auth: %s\n", status)
	default:
		fmt.Fprintln(out, "provider_auth: unknown")
	}
	fmt.Fprintf(out, "model: %s\n", cfg.ModelID)
	fmt.Fprintf(out, "session_dir: %s\n", sessionDir)
	fmt.Fprintf(out, "skills_disabled: %t\n", cfg.Prompt.NoSkills)
	return cfg.Validate()
}

func runAgent(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cmd command.Run,
	cfg config.Runtime,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
	profile prompt.Profile,
) error {
	if cmd.Config.ServerURL != "" {
		_, err := httpapi.RenderAGUIRun(ctx, http.DefaultClient, cmd.Config.ServerURL, httpapi.RunRequest{
			Prompt:         cmd.Prompt,
			Session:        sessionOptions(cmd.Session),
			GatewaySession: cmd.Config.GatewaySession,
		}, out)
		if err != nil {
			return wrapError("model", err)
		}
		return nil
	}
	if cmd.Config.GatewaySession != "" {
		client := httpapi.NewUnixSocketClient(observeSocketPath(cwd, cmd.Config.ObserveSocket))
		_, err := httpapi.RenderAGUIRun(ctx, client, "http://fritz", httpapi.RunRequest{
			Prompt:         cmd.Prompt,
			Session:        sessionOptions(cmd.Session),
			GatewaySession: cmd.Config.GatewaySession,
		}, out)
		if err != nil {
			return wrapError("model", err)
		}
		return nil
	}
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newClient, registry, profile)
	runtime, err := service.NewRuntime(ctx, agent.RuntimeOptions{
		Session: session.StartOptions{
			Continue:    cmd.Session.Continue,
			SessionPath: cmd.Session.Session,
			ForkPath:    cmd.Session.Fork,
			NoSession:   cmd.Session.NoSession,
			NewSession:  cmd.Session.NewSession,
		},
	})
	if err != nil {
		return wrapError("session", err)
	}
	handle, err := runtime.Submit(ctx, agent.RunRequest{Prompt: cmd.Prompt})
	if err != nil {
		return wrapError("prompt", err)
	}
	if _, err := renderLocalRun(out, handle); err != nil {
		return wrapError("model", err)
	}
	return nil
}

func runChat(
	ctx context.Context,
	in io.Reader,
	out io.Writer,
	cwd string,
	cmd command.Chat,
	cfg config.Runtime,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
	profile prompt.Profile,
) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	if cmd.Config.ServerURL != "" {
		return runRemoteChat(ctx, scanner, out, cmd)
	}
	if cmd.Config.GatewaySession != "" {
		if isInteractiveTTY(in, out) {
			runtime := newRemoteTerminalRuntime(httpapi.NewUnixSocketClient(observeSocketPath(cwd, cmd.Config.ObserveSocket)), "http://fritz", cmd.Config.GatewaySession, cfg.ModelID)
			initialState, err := runtime.InitialState(ctx)
			if err != nil {
				return wrapError("session", err)
			}
			if err := terminalui.RunWithState(ctx, in, out, runtime, initialState); err != nil {
				return wrapError("input", err)
			}
			return nil
		}
		return runRemoteChatWithClient(ctx, scanner, out, cmd, httpapi.NewUnixSocketClient(observeSocketPath(cwd, cmd.Config.ObserveSocket)), "http://fritz")
	}
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newClient, registry, profile)
	runtime, err := service.NewRuntime(ctx, agent.RuntimeOptions{
		Session: session.StartOptions{
			Continue:    cmd.Session.Continue,
			SessionPath: cmd.Session.Session,
			ForkPath:    cmd.Session.Fork,
			NoSession:   cmd.Session.NoSession,
			NewSession:  cmd.Session.NewSession,
		},
	})
	if err != nil {
		return wrapError("session", err)
	}
	if isInteractiveTTY(in, out) {
		if err := terminalui.Run(ctx, in, out, runtime); err != nil {
			return wrapError("input", err)
		}
		return nil
	}

	if cfg.Chat.ShowHelpOnStart {
		fmt.Fprintln(out, "chat commands:")
		fmt.Fprintln(out, "  :help")
		fmt.Fprintln(out, "  :reset")
		fmt.Fprintln(out, "  :quit")
	}

	for scanner.Scan() {
		switch input := chat.ParseInput(scanner.Text()); input.Kind {
		case chat.InputEmpty:
			continue
		case chat.InputHelp:
			fmt.Fprintln(out, "chat commands:")
			fmt.Fprintln(out, "  :help")
			fmt.Fprintln(out, "  :reset")
			fmt.Fprintln(out, "  :quit")
		case chat.InputReset:
			runtime.Reset()
			fmt.Fprintln(out, "history cleared")
		case chat.InputQuit:
			return nil
		case chat.InputPrompt:
			handle, err := runtime.Submit(ctx, agent.RunRequest{Prompt: input.Text})
			if err != nil {
				return wrapError("prompt", err)
			}
			if _, err := renderLocalRun(out, handle); err != nil {
				return wrapError("model", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return wrapError("input", fmt.Errorf("read local chat input: %w", err))
	}

	return nil
}

func isInteractiveTTY(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok {
		return false
	}
	outFile, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(inFile.Fd())) && term.IsTerminal(int(outFile.Fd()))
}

func printAgentUsage(out io.Writer) {
	fmt.Fprintln(out, brand.CLIName)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "usage:")
	fmt.Fprintf(out, "  %s help\n", brand.CLIName)
	fmt.Fprintf(out, "  %s doctor\n", brand.CLIName)
	fmt.Fprintf(out, "  %s run <prompt>\n", brand.CLIName)
	fmt.Fprintf(out, "  %s chat\n", brand.CLIName)
	fmt.Fprintf(out, "  %s serve\n", brand.CLIName)
	fmt.Fprintf(out, "  %s attach [run-id]\n", brand.CLIName)
	fmt.Fprintf(out, "  %s auth login <provider>\n", brand.CLIName)
	fmt.Fprintf(out, "  %s auth logout <provider>\n", brand.CLIName)
	fmt.Fprintf(out, "  %s auth status [provider]\n", brand.CLIName)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "examples:")
	fmt.Fprintf(out, "  %s run %q\n", brand.CLIName, "summarize README.md")
	fmt.Fprintf(out, "  %s run %q\n", brand.CLIName, "/skill:task-pack-create add tickets")
	fmt.Fprintf(out, "  %s --continue chat\n", brand.CLIName)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "config flags:")
	fmt.Fprintln(out, "  --config <path>")
	fmt.Fprintln(out, "  --provider <name>")
	fmt.Fprintln(out, "  --model <id>")
	fmt.Fprintln(out, "  --gemini-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-auth-base-url <url>")
	fmt.Fprintln(out, "  --openai-codex-client-id <id>")
	fmt.Fprintln(out, "  --openai-codex-originator <value>")
	fmt.Fprintln(out, "  --openai-codex-redirect-url <url>")
	fmt.Fprintln(out, "  --log-file <path>")
	fmt.Fprintln(out, "  --log-level <level>")
	fmt.Fprintln(out, "  --server <url>")
	fmt.Fprintln(out, "  --observe-socket <path>")
	fmt.Fprintln(out, "  --gateway-session <key>")
	fmt.Fprintln(out, "  --listen <addr>")
	fmt.Fprintln(out, "  --session-dir <path>")
	fmt.Fprintln(out, "  --chat-help=<bool>")
	fmt.Fprintln(out, "  --auto-compact=<bool>")
	fmt.Fprintln(out, "  --compact-threshold <n>")
	fmt.Fprintln(out, "  --compact-keep <n>")
	fmt.Fprintln(out, "  --command-timeout <duration>")
	fmt.Fprintln(out, "  --skill <path>")
	fmt.Fprintln(out, "  --no-skills")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "session flags:")
	fmt.Fprintln(out, "  --continue, -c")
	fmt.Fprintln(out, "  --session <path>")
	fmt.Fprintln(out, "  --fork <path>")
	fmt.Fprintln(out, "  --new-session")
	fmt.Fprintln(out, "  --no-session")
}

func printGatewayUsage(out io.Writer) {
	fmt.Fprintln(out, "fritz-telegram")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  fritz-telegram [--poll-once]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "config flags:")
	fmt.Fprintln(out, "  --telegram-bot-token <token>")
	fmt.Fprintln(out, "  --telegram-endpoint <url>")
	fmt.Fprintln(out, "  --telegram-poll-timeout <duration>")
	fmt.Fprintln(out, "  --telegram-pairing-token <token>")
	fmt.Fprintln(out, "  --telegram-allow-user <id>")
	fmt.Fprintln(out, "  --telegram-training-db <path>")
	fmt.Fprintln(out, "  --heartbeat=<bool>")
	fmt.Fprintln(out, "  --heartbeat-interval <duration>")
	fmt.Fprintln(out, "  --log-file <path>")
	fmt.Fprintln(out, "  --log-level <level>")
	fmt.Fprintln(out, "  --observe-socket <path>")
	fmt.Fprintln(out, "  --gateway-session <key>")
	fmt.Fprintln(out, "  --provider <name>")
	fmt.Fprintln(out, "  --model <id>")
	fmt.Fprintln(out, "  --gemini-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-auth-base-url <url>")
	fmt.Fprintln(out, "  --openai-codex-client-id <id>")
	fmt.Fprintln(out, "  --openai-codex-originator <value>")
	fmt.Fprintln(out, "  --openai-codex-redirect-url <url>")
}

func printSlackUsage(out io.Writer) {
	fmt.Fprintln(out, "fritz-slack")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  fritz-slack")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "config flags:")
	fmt.Fprintln(out, "  --slack-bot-token <token>")
	fmt.Fprintln(out, "  --slack-app-token <token>")
	fmt.Fprintln(out, "  --slack-endpoint <url>")
	fmt.Fprintln(out, "  --slack-allow-user <id>")
	fmt.Fprintln(out, "  --slack-allow-channel <id>")
	fmt.Fprintln(out, "  --slack-assistant=<bool>")
	fmt.Fprintln(out, "  --heartbeat=<bool>")
	fmt.Fprintln(out, "  --heartbeat-interval <duration>")
	fmt.Fprintln(out, "  --log-file <path>")
	fmt.Fprintln(out, "  --log-level <level>")
	fmt.Fprintln(out, "  --provider <name>")
	fmt.Fprintln(out, "  --model <id>")
	fmt.Fprintln(out, "  --gemini-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-endpoint <url>")
	fmt.Fprintln(out, "  --openai-codex-auth-base-url <url>")
	fmt.Fprintln(out, "  --openai-codex-client-id <id>")
	fmt.Fprintln(out, "  --openai-codex-originator <value>")
	fmt.Fprintln(out, "  --openai-codex-redirect-url <url>")
}

func configSourceFromCommand(options command.ConfigOptions) config.Source {
	return config.Source{
		Provider:               options.Provider,
		ModelID:                options.ModelID,
		GeminiEndpoint:         options.GeminiEndpoint,
		OpenAICodexEndpoint:    options.OpenAICodexEndpoint,
		OpenAICodexAuthBaseURL: options.OpenAICodexAuthBaseURL,
		OpenAICodexClientID:    options.OpenAICodexClientID,
		OpenAICodexOriginator:  options.OpenAICodexOriginator,
		OpenAICodexRedirectURL: options.OpenAICodexRedirectURL,
		Telegram: config.TelegramConfigSource{
			BotToken:       options.TelegramBotToken,
			Endpoint:       options.TelegramEndpoint,
			PollTimeout:    options.TelegramPollTimeout,
			PairingToken:   options.TelegramPairingToken,
			AllowedUsers:   append([]string(nil), options.TelegramAllowedUsers...),
			TrainingDBPath: options.TelegramTrainingDBPath,
		},
		Slack: config.SlackConfigSource{
			BotToken:        options.SlackBotToken,
			AppToken:        options.SlackAppToken,
			Endpoint:        options.SlackEndpoint,
			AllowedUsers:    append([]string(nil), options.SlackAllowedUsers...),
			AllowedChannels: append([]string(nil), options.SlackAllowedChannels...),
			Assistant:       options.SlackAssistantEnabled,
		},
		Heartbeat: config.HeartbeatConfigSource{
			Enabled:  options.HeartbeatEnabled,
			Interval: options.HeartbeatInterval,
		},
		Log: config.LogConfigSource{
			File:  options.LogFile,
			Level: options.LogLevel,
		},
		Chat: config.ChatConfigSource{
			ShowHelpOnStart: options.ChatHelp,
		},
		Session: config.SessionConfigSource{
			Dir:                    options.SessionDir,
			AutoCompact:            options.AutoCompact,
			CompactThresholdTurns:  options.CompactThresholdTurns,
			CompactKeepTurns:       options.CompactKeepTurns,
			CompactThresholdTokens: options.CompactThresholdTokens,
			CompactTargetTokens:    options.CompactTargetTokens,
		},
		Prompt: config.PromptConfigSource{
			NoSkills:   options.NoSkills,
			SkillPaths: append([]string(nil), options.SkillPaths...),
		},
		Runtime: config.RuntimeConfigSource{
			CommandTimeout: options.CommandTimeout,
		},
	}
}

func providerEndpoint(cfg config.Runtime) string {
	switch cfg.Provider {
	case provider.OpenAICodex:
		return cfg.OpenAICodexEndpoint
	default:
		return cfg.GeminiEndpoint
	}
}

func runAuthLogin(ctx context.Context, in io.Reader, out io.Writer, cwd string, cfg config.Runtime, cmd command.AuthLogin) error {
	kind, err := provider.Parse(cmd.Provider)
	if err != nil {
		return err
	}
	switch kind {
	case provider.OpenAICodex:
		return runOpenAICodexLogin(ctx, in, out, cwd, cfg)
	default:
		return fmt.Errorf("auth login unsupported for provider %q", kind)
	}
}

func runAuthLogout(out io.Writer, cwd string, cfg config.Runtime, cmd command.AuthLogout) error {
	kind, err := provider.Parse(cmd.Provider)
	if err != nil {
		return err
	}
	deleted, err := newAuthStore().Delete(kind)
	if err != nil {
		return err
	}
	if deleted {
		fmt.Fprintf(out, "auth removed: %s\n", kind)
		return nil
	}
	fmt.Fprintf(out, "auth missing: %s\n", kind)
	return nil
}

func runAuthStatus(out io.Writer, cwd string, cfg config.Runtime, cmd command.AuthStatus) error {
	store := newAuthStore()
	if strings.TrimSpace(cmd.Provider) != "" {
		return printAuthStatus(out, store, cfg, cmd.Provider)
	}
	entries, err := store.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "auth: none")
	} else {
		for _, entry := range entries {
			fmt.Fprintf(out, "auth: %s %s\n", entry.Provider, entry.Kind)
		}
	}
	if cfg.HasGeminiAPIKey() {
		fmt.Fprintln(out, "auth: gemini env_api_key")
	}
	return nil
}

func printAuthStatus(out io.Writer, store authstore.Store, cfg config.Runtime, providerName string) error {
	kind, err := provider.Parse(providerName)
	if err != nil {
		return err
	}
	if kind == provider.Gemini && cfg.HasGeminiAPIKey() {
		fmt.Fprintln(out, "provider: gemini")
		fmt.Fprintln(out, "status: env_api_key")
		return nil
	}
	entry, ok, err := store.Get(kind)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "provider: %s\n", kind)
	if !ok {
		fmt.Fprintln(out, "status: missing")
		return nil
	}
	fmt.Fprintf(out, "status: %s\n", authstore.FormatStatus(entry))
	return nil
}

func runOpenAICodexLogin(ctx context.Context, in io.Reader, out io.Writer, cwd string, cfg config.Runtime) error {
	oauthCfg := openaicodex.OAuthConfigFromRuntime(cfg)
	flow, err := createOpenAICodexAuthorizationFlow(oauthCfg)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "open browser or open this url:")
	fmt.Fprintln(out, flow.URL)
	_ = openBrowserURL(ctx, flow.URL)

	var code string
	server, err := startOpenAICodexCallbackServer(oauthCfg, flow.State)
	if err == nil {
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		defer server.Close(context.Background())
		code, err = server.Wait(waitCtx)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	if code == "" {
		fmt.Fprintln(out, "paste redirect url or auth code:")
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		code, err = openaicodex.ValidateAuthorizationInput(openaicodex.ParseAuthorizationInput(line), flow.State)
		if err != nil {
			return err
		}
	}

	creds, err := exchangeOpenAICodexAuthorizationCode(ctx, oauthCfg, code, flow.Verifier)
	if err != nil {
		return err
	}
	if err := newAuthStore().PutOAuth(provider.OpenAICodex, creds); err != nil {
		return err
	}
	fmt.Fprintln(out, "auth stored: openai-codex")
	return nil
}

func tryOpenBrowser(ctx context.Context, target string) error {
	candidates := [][]string{
		{"xdg-open", target},
		{"open", target},
	}
	for _, cmd := range candidates {
		path, err := exec.LookPath(cmd[0])
		if err != nil {
			continue
		}
		return exec.CommandContext(ctx, path, cmd[1:]...).Start()
	}
	return errors.New("no browser opener found")
}

func newService(cwd string, cfg config.Runtime, newClient func(cfg config.Runtime) model.Client, registry *tool.Registry, profile prompt.Profile) *agent.Service {
	return agent.NewServiceWithPromptProfile(cwd, cfg, newClient, func(cfg config.Runtime) *tool.Registry {
		if registry != nil {
			return registry
		}
		return toolRegistryForProfile(cfg, profile)
	}, profile)
}

func validateProviderAccess(ctx context.Context, cwd string, cfg config.Runtime) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	_, err := auth.NewResolver(newAuthStore()).Resolve(ctx, cfg)
	return err
}

func configureLogger(cwd string, cfg config.Runtime) (func(), error) {
	path := config.ResolveLogFile(cwd, cfg.Log.File)
	closeFn, err := logx.Configure(logx.Config{
		Path:  path,
		Level: cfg.Log.Level,
	})
	if err != nil {
		return nil, err
	}
	logger := logx.Component("app")
	logger.Info().
		Str("event", "app.logger.configured").
		Str("path", logx.Path()).
		Str("log_level", cfg.Log.Level).
		Msg("")
	return func() {
		_ = closeFn()
	}, nil
}

func runTelegram(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cmd command.Telegram,
	cfg config.Runtime,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
) error {
	_ = out
	if err := cfg.ValidateTelegram(); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newClient, registry, prompt.ProfileGateway)
	hub := observe.NewHub(1024, 50)
	observedService := observe.WrapService(engine.WrapService(service), hub)
	gw := ingressruntime.New(cwd, cfg, observedService)
	if socketPath := observeSocketPath(cwd, cmd.Config.ObserveSocket); socketPath != "" && !cmd.PollOnce {
		go func() {
			_ = serveObserveSocket(ctx, socketPath, observedService, hub, gw)
		}()
	}
	client := telegram.NewHTTPClient(http.DefaultClient, cfg.Telegram.Endpoint, cfg.Telegram.BotToken)
	var training telegram.TrainingProvider
	if path := strings.TrimSpace(cfg.Telegram.TrainingDBPath); path != "" {
		training = trainingplan.NewReader(path)
	}
	adapter := telegram.NewAdapterWithPaths(gw.Paths(), client, gw, telegram.Config{
		PollTimeout:  cfg.Telegram.PollTimeout,
		PairingToken: cfg.Telegram.PairingToken,
		AllowedUsers: append([]string(nil), cfg.Telegram.AllowedUsers...),
		Training:     training,
		Transcriber: transcriptiongemini.NewClient(
			cfg.GeminiAPIKey,
			transcriptiongemini.WithEndpoint(cfg.GeminiEndpoint),
		),
	})
	var manager *heartbeat.Manager
	var err error
	if cfg.Heartbeat.Enabled {
		source := heartbeat.CombineSources(
			newHeartbeatSource(cfg),
			reminderwake.New(gw.Paths().ReminderPath, gw.Paths().RoutingSessionMapPath),
		)
		manager, err = heartbeat.NewManager(
			cfg.Heartbeat.Interval,
			source,
			heartbeat.SessionRunner{
				Service:      observedService,
				SessionPaths: gw.SessionPathForKey,
				Prompt:       "Heartbeat check. If there is nothing actionable to do right now, reply exactly HEARTBEAT_OK.",
				PromptForWake: func(wake heartbeat.Wake) string {
					if strings.TrimSpace(wake.Reason) == "" {
						return "Heartbeat check. If there is nothing actionable to do right now, reply exactly HEARTBEAT_OK."
					}
					return "A reminder or scheduled task is due now.\n\n" + strings.TrimSpace(wake.Reason) + "\n\nReply only with the concrete user-facing reminder to send now. If nothing should be sent, reply exactly HEARTBEAT_OK."
				},
			},
			telegram.NewSender(client),
			heartbeat.NewJSONStoreForPaths(gw.Paths()),
		)
		if err != nil {
			return wrapError("heartbeat", err)
		}
	}
	if cmd.PollOnce {
		if manager != nil {
			if err := manager.Tick(ctx); err != nil {
				return wrapError("heartbeat", err)
			}
		}
		_, err := adapter.PollOnce(ctx)
		return wrapError("telegram", err)
	}
	if manager != nil {
		go func() {
			_ = manager.Run(ctx)
		}()
	}
	return wrapError("telegram", adapter.Run(ctx))
}

func runAttach(ctx context.Context, out io.Writer, cwd string, cmd command.Attach) error {
	socketPath := observeSocketPath(cwd, cmd.Config.ObserveSocket)
	if socketPath == "" {
		return wrapError("observe", errors.New("missing observe socket"))
	}
	client := httpapi.NewUnixSocketClient(socketPath)
	baseURL := "http://fritz"
	runID := strings.TrimSpace(cmd.RunID)
	if runID == "" {
		runs, err := httpapi.ListRuns(ctx, client, baseURL)
		if err != nil {
			return wrapError("observe", err)
		}
		active := make([]observe.RunInfo, 0, len(runs))
		for _, run := range runs {
			if run.Status == observe.StatusRunning {
				active = append(active, run)
			}
		}
		if len(active) == 1 {
			runID = active[0].ID
		} else {
			for _, run := range runs {
				fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", run.ID, run.Status, run.Session.Path, run.Prompt)
			}
			if len(active) == 0 {
				return nil
			}
			return wrapError("observe", fmt.Errorf("multiple active runs; pass a run id"))
		}
	}
	if err := httpapi.AttachRun(ctx, client, baseURL, runID, out); err != nil {
		return wrapError("observe", err)
	}
	return nil
}

func serveObserveSocket(ctx context.Context, socketPath string, service engine.Service, hub *observe.Hub, resolver httpapi.SessionResolver) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	_ = os.Chmod(socketPath, 0o600)
	server := &http.Server{
		Handler: httpapi.NewHandlerWithOptions(service, hub, resolver),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	err = server.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func observeSocketPath(cwd string, override string) string {
	if path := strings.TrimSpace(override); path != "" {
		return path
	}
	if path := strings.TrimSpace(os.Getenv("FRITZ_OBSERVE_SOCKET")); path != "" {
		return path
	}
	return filepath.Join(config.DefaultWorkspaceGatewayRoot(cwd), "observe.sock")
}

func renderLocalRun(out io.Writer, handle agent.RunHandle) (agent.RunResult, error) {
	var streamed bool
	for event := range handle.Events {
		switch event.Kind {
		case agent.EventTextDelta:
			streamed = true
			if _, err := io.WriteString(out, event.TextDelta); err != nil {
				return agent.RunResult{}, err
			}
		case agent.EventMessageCompleted:
			text := ""
			if event.Message != nil {
				text = event.Message.Text()
			}
			if streamed {
				if _, err := io.WriteString(out, "\n"); err != nil {
					return agent.RunResult{}, err
				}
				streamed = false
			} else if text != "" {
				if _, err := fmt.Fprintln(out, text); err != nil {
					return agent.RunResult{}, err
				}
			}
		}
	}
	result := <-handle.Done
	return result, result.Err
}

func runRemoteChat(
	ctx context.Context,
	scanner *bufio.Scanner,
	out io.Writer,
	cmd command.Chat,
) error {
	return runRemoteChatWithClient(ctx, scanner, out, cmd, http.DefaultClient, cmd.Config.ServerURL)
}

func runRemoteChatWithClient(
	ctx context.Context,
	scanner *bufio.Scanner,
	out io.Writer,
	cmd command.Chat,
	client *http.Client,
	baseURL string,
) error {
	sessionReq := sessionOptions(cmd.Session)
	if baseURL == "" {
		return nil
	}
	if sessionReq.SessionPath == "" && cmd.Session.Session != "" {
		sessionReq.SessionPath = cmd.Session.Session
	}
	if cmd.Config.ChatHelp == nil || *cmd.Config.ChatHelp {
		fmt.Fprintln(out, "chat commands:")
		fmt.Fprintln(out, "  :help")
		fmt.Fprintln(out, "  :reset")
		fmt.Fprintln(out, "  :quit")
	}
	for scanner.Scan() {
		switch input := chat.ParseInput(scanner.Text()); input.Kind {
		case chat.InputEmpty:
			continue
		case chat.InputHelp:
			fmt.Fprintln(out, "chat commands:")
			fmt.Fprintln(out, "  :help")
			fmt.Fprintln(out, "  :reset")
			fmt.Fprintln(out, "  :quit")
		case chat.InputReset:
			sessionReq = sessionOptions(command.SessionOptions{NewSession: true})
			fmt.Fprintln(out, "history cleared")
		case chat.InputQuit:
			return nil
		case chat.InputPrompt:
			summary, err := httpapi.RenderAGUIRun(ctx, client, baseURL, httpapi.RunRequest{
				Prompt:         input.Text,
				Session:        sessionReq,
				GatewaySession: cmd.Config.GatewaySession,
			}, out)
			if err != nil {
				return err
			}
			if summary.SessionPath != "" {
				sessionReq = httpapi.SessionStartOptions{SessionPath: summary.SessionPath}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read remote chat input: %w", err)
	}
	return nil
}

func sessionOptions(options command.SessionOptions) httpapi.SessionStartOptions {
	return httpapi.SessionStartOptions{
		Continue:    options.Continue,
		SessionPath: options.Session,
		ForkPath:    options.Fork,
		NoSession:   options.NoSession,
		NewSession:  options.NewSession,
	}
}

type remoteTerminalRuntime struct {
	client         *http.Client
	baseURL        string
	gatewaySession string
	modelID        string
	seq            atomic.Uint64
}

func newRemoteTerminalRuntime(client *http.Client, baseURL string, gatewaySession string, modelID string) *remoteTerminalRuntime {
	return &remoteTerminalRuntime{
		client:         client,
		baseURL:        baseURL,
		gatewaySession: gatewaySession,
		modelID:        modelID,
	}
}

func (r *remoteTerminalRuntime) Submit(ctx context.Context, req agent.RunRequest) (agent.RunHandle, error) {
	runID := fmt.Sprintf("remote-%06d", r.seq.Add(1))
	events := make(chan agent.Event, 64)
	done := make(chan agent.RunResult, 1)
	go func() {
		defer close(events)
		defer close(done)
		state := chat.NewState()
		err := httpapi.StreamRunPayloads(ctx, r.client, r.baseURL, httpapi.RunRequest{
			Prompt:         req.Prompt,
			GatewaySession: r.gatewaySession,
		}, func(payload map[string]any) error {
			event, ok := aguiPayloadToAgentEvent(runID, payload)
			if !ok {
				return nil
			}
			if event.RunID != "" {
				runID = event.RunID
			}
			events <- event
			return nil
		})
		done <- agent.RunResult{State: state, Err: err}
	}()
	return agent.RunHandle{ID: runID, Events: events, Done: done}, nil
}

func (r *remoteTerminalRuntime) Reset() {}

func (r *remoteTerminalRuntime) CancelRun(runID string) bool {
	if err := httpapi.CancelRun(context.Background(), r.client, r.baseURL, runID); err != nil {
		return false
	}
	return true
}

func (r *remoteTerminalRuntime) ModelID() string {
	if strings.TrimSpace(r.modelID) == "" {
		return "remote"
	}
	return r.modelID
}

func (r *remoteTerminalRuntime) InitialState(ctx context.Context) (terminalui.State, error) {
	state := terminalui.NewState()
	session, err := httpapi.GetGatewaySession(ctx, r.client, r.baseURL, r.gatewaySession)
	if err != nil {
		return state, err
	}
	for _, turn := range session.Transcript {
		state = state.AddUserPrompt(turn.User)
		message := model.TextMessage(model.ModelRole, turn.Assistant)
		state = state.Apply(agent.Event{
			ID:        fmt.Sprintf("history-%d", time.Now().UnixNano()),
			Kind:      agent.EventMessageCompleted,
			MessageID: fmt.Sprintf("history-assistant-%d", time.Now().UnixNano()),
			Message:   &message,
			Time:      time.Now().UTC(),
		})
	}
	return state, nil
}

func aguiPayloadToAgentEvent(fallbackRunID string, payload map[string]any) (agent.Event, bool) {
	typ, _ := payload["type"].(string)
	runID, _ := payload["runId"].(string)
	if runID == "" {
		runID = fallbackRunID
	}
	messageID, _ := payload["messageId"].(string)
	event := agent.Event{
		ID:        fmt.Sprintf("%s-%d", runID, time.Now().UnixNano()),
		RunID:     runID,
		MessageID: messageID,
		Time:      time.Now().UTC(),
	}
	switch typ {
	case "RUN_STARTED":
		event.Kind = agent.EventRunStarted
	case "REASONING_MESSAGE_START":
		event.Kind = agent.EventReasoningStarted
	case "REASONING_MESSAGE_CONTENT":
		event.Kind = agent.EventReasoningDelta
		event.TextDelta, _ = payload["delta"].(string)
	case "REASONING_MESSAGE_END":
		event.Kind = agent.EventReasoningCompleted
	case "TEXT_MESSAGE_CONTENT":
		event.Kind = agent.EventTextDelta
		event.TextDelta, _ = payload["delta"].(string)
	case "TEXT_MESSAGE_END":
		event.Kind = agent.EventMessageCompleted
		text, _ := payload["text"].(string)
		message := model.TextMessage(model.ModelRole, text)
		event.Message = &message
	case "TOOL_CALL_START":
		event.Kind = agent.EventToolCallStarted
		event.ToolCall = &tool.Call{
			ID:   stringFromPayload(payload, "toolCallId", "remote-tool"),
			Name: stringFromPayload(payload, "toolName", "tool"),
		}
	case "TOOL_CALL_ARGS":
		return agent.Event{}, false
	case "TOOL_CALL_RESULT":
		event.Kind = agent.EventToolCallCompleted
		callID := stringFromPayload(payload, "toolCallId", "remote-tool")
		toolName := stringFromPayload(payload, "toolName", "tool")
		event.ToolCall = &tool.Call{ID: callID, Name: toolName}
		event.ToolResult = aguiToolResult(toolName, payload["result"])
	case "RUN_ERROR":
		event.Kind = agent.EventRunFailed
		event.Error, _ = payload["message"].(string)
	case "RUN_FINISHED":
		event.Kind = agent.EventRunFinished
	default:
		return agent.Event{}, false
	}
	return event, true
}

func aguiToolResult(name string, raw any) *tool.Result {
	result := tool.Result{Name: name}
	if rawMap, ok := raw.(map[string]any); ok {
		if isError, ok := rawMap["isError"].(bool); ok {
			result.IsError = isError
		}
		if parts, ok := rawMap["parts"].([]any); ok {
			for _, part := range parts {
				partMap, ok := part.(map[string]any)
				if !ok {
					continue
				}
				if text, ok := partMap["text"].(string); ok {
					result.Parts = append(result.Parts, tool.TextPart(text))
				}
			}
		}
	}
	return &result
}

func stringFromPayload(payload map[string]any, key string, fallback string) string {
	value, _ := payload[key].(string)
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func runServe(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cmd command.Serve,
	cfg config.Runtime,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
	profile prompt.Profile,
) error {
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	listen := cmd.Config.ListenAddr
	if listen == "" {
		listen = ":8080"
	}
	service := newService(cwd, cfg, newClient, registry, profile)
	server := &http.Server{
		Addr:    listen,
		Handler: httpapi.NewHandler(engine.WrapService(service)),
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	fmt.Fprintf(out, "listening on %s\n", listen)
	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func runTelegramProcess(
	ctx context.Context,
	args []string,
	out io.Writer,
	cwd string,
	newClient func(config.Runtime) model.Client,
	registry *tool.Registry,
) error {
	if len(args) == 1 {
		switch args[0] {
		case "help", "--help", "-h":
			printGatewayUsage(out)
			return nil
		}
	}
	cmd, err := command.ParseTelegramProcess(args)
	if err != nil {
		return wrapError("command", err)
	}
	cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
	if err != nil {
		return wrapError("config", err)
	}
	closeLogger, err := configureLogger(cwd, cfg)
	if err != nil {
		return wrapError("config", err)
	}
	defer closeLogger()
	return runTelegram(ctx, out, cwd, cmd, cfg, newClient, registry)
}

func runSlack(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cfg config.Runtime,
	newClient func(cfg config.Runtime) model.Client,
	registry *tool.Registry,
) error {
	_ = out
	if err := cfg.ValidateSlack(); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newClient, registry, prompt.ProfileGateway)
	gw := ingressruntime.New(cwd, cfg, engine.WrapService(service))
	client := slack.NewHTTPClient(http.DefaultClient, cfg.Slack.Endpoint, cfg.Slack.BotToken, cfg.Slack.AppToken)
	adapter := slack.NewAdapterWithPaths(gw.Paths(), client, gw, slack.Config{
		AllowedUsers:    append([]string(nil), cfg.Slack.AllowedUsers...),
		AllowedChannels: append([]string(nil), cfg.Slack.AllowedChannels...),
		Assistant:       cfg.Slack.Assistant,
	})
	var manager *heartbeat.Manager
	var err error
	if cfg.Heartbeat.Enabled {
		source := heartbeat.CombineSources(
			newHeartbeatSource(cfg),
			reminderwake.New(gw.Paths().ReminderPath, gw.Paths().RoutingSessionMapPath),
		)
		manager, err = heartbeat.NewManager(
			cfg.Heartbeat.Interval,
			source,
			heartbeat.SessionRunner{
				Service:      engine.WrapService(service),
				SessionPaths: gw.SessionPathForKey,
				Prompt:       "Heartbeat check. If there is nothing actionable to do right now, reply exactly HEARTBEAT_OK.",
				PromptForWake: func(wake heartbeat.Wake) string {
					if strings.TrimSpace(wake.Reason) == "" {
						return "Heartbeat check. If there is nothing actionable to do right now, reply exactly HEARTBEAT_OK."
					}
					return "A reminder or scheduled task is due now.\n\n" + strings.TrimSpace(wake.Reason) + "\n\nReply only with the concrete user-facing reminder to send now. If nothing should be sent, reply exactly HEARTBEAT_OK."
				},
			},
			slack.NewSender(client),
			heartbeat.NewJSONStoreForPaths(gw.Paths()),
		)
		if err != nil {
			return wrapError("heartbeat", err)
		}
	}
	if manager != nil {
		go func() {
			_ = manager.Run(ctx)
		}()
	}
	return wrapError("slack", adapter.Run(ctx))
}

func runSlackProcess(
	ctx context.Context,
	args []string,
	out io.Writer,
	cwd string,
	newClient func(config.Runtime) model.Client,
	registry *tool.Registry,
) error {
	if len(args) == 1 {
		switch args[0] {
		case "help", "--help", "-h":
			printSlackUsage(out)
			return nil
		}
	}
	cmd, err := command.ParseSlackProcess(args)
	if err != nil {
		return wrapError("command", err)
	}
	cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
	if err != nil {
		return wrapError("config", err)
	}
	closeLogger, err := configureLogger(cwd, cfg)
	if err != nil {
		return wrapError("config", err)
	}
	defer closeLogger()
	return runSlack(ctx, out, cwd, cfg, newClient, registry)
}

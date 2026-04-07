package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"fritz/internal/agent"
	"fritz/internal/auth"
	"fritz/internal/authstore"
	"fritz/internal/brand"
	"fritz/internal/chat"
	"fritz/internal/command"
	"fritz/internal/config"
	"fritz/internal/engine"
	gatewayruntime "fritz/internal/gateway"
	"fritz/internal/gemini"
	"fritz/internal/heartbeat"
	"fritz/internal/httpapi"
	"fritz/internal/logx"
	"fritz/internal/model"
	"fritz/internal/openaicodex"
	"fritz/internal/prompt"
	"fritz/internal/provider"
	"fritz/internal/reminderwake"
	"fritz/internal/session"
	"fritz/internal/telegram"
	"fritz/internal/terminalui"
	"fritz/internal/tool"
	transcriptiongemini "fritz/internal/transcription/gemini"
	"golang.org/x/term"
)

func Run(ctx context.Context, args []string) error {
	cwd := mustGetwd()
	return runWithProfile(ctx, args, os.Stdin, os.Stdout, func(cfg config.Runtime) model.Gateway {
		return defaultGatewayFactory(cwd, cfg)
	}, nil, prompt.ProfileCoding)
}

var newHeartbeatSource = func(config.Runtime) heartbeat.Source {
	return heartbeat.NullSource{}
}

var createOpenAICodexAuthorizationFlow = openaicodex.CreateAuthorizationFlow
var openBrowserURL = tryOpenBrowser
var startOpenAICodexCallbackServer = openaicodex.StartCallbackServer
var exchangeOpenAICodexAuthorizationCode = openaicodex.ExchangeAuthorizationCode

func defaultGatewayFactory(cwd string, cfg config.Runtime) model.Gateway {
	switch cfg.Provider {
	case provider.OpenAICodex:
		resolver := auth.NewResolver(authstore.NewGlobalFileStore())
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

func RunWithGateway(ctx context.Context, args []string) error {
	cwd := mustGetwd()
	return runWithProfile(ctx, args, os.Stdin, os.Stdout, func(cfg config.Runtime) model.Gateway {
		return defaultGatewayFactory(cwd, cfg)
	}, nil, prompt.ProfileGateway)
}

func run(
	ctx context.Context,
	args []string,
	in io.Reader,
	out io.Writer,
	newGateway func(cfg config.Runtime) model.Gateway,
	registry *tool.Registry,
) error {
	return runWithProfile(ctx, args, in, out, newGateway, registry, prompt.ProfileCoding)
}

func runWithProfile(
	ctx context.Context,
	args []string,
	in io.Reader,
	out io.Writer,
	newGateway func(cfg config.Runtime) model.Gateway,
	registry *tool.Registry,
	profile prompt.Profile,
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
		printUsage(out)
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
		return runAgent(ctx, out, cwd, cmd, cfg, newGateway, registry, profile)
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
		return runChat(ctx, in, out, cwd, cmd, cfg, newGateway, registry, profile)
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
		return runServe(ctx, out, cwd, cmd, cfg, newGateway, registry, profile)
	case command.Telegram:
		cfg, err := resolveRuntimeConfig(cwd, cmd.Config)
		if err != nil {
			return wrapError("config", err)
		}
		closeLogger, err := configureLogger(cwd, cfg)
		if err != nil {
			return wrapError("config", err)
		}
		defer closeLogger()
		return runTelegram(ctx, out, cwd, cmd, cfg, newGateway, registry)
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
	return defaultToolRegistryForConfig(config.Resolve(config.Sources{
		Defaults: config.DefaultSource(),
	}))
}

func defaultToolRegistryForConfig(cfg config.Runtime) *tool.Registry {
	registry := tool.NewRegistry()
	root := mustGetwd()
	registry.Register(tool.NewBashTool(root, tool.WithDefaultTimeout(cfg.CommandTimeout)))
	registry.Register(tool.NewEditTool(root, 128*1024))
	registry.Register(tool.NewFindTool(root))
	registry.Register(tool.NewGrepTool(root))
	registry.Register(tool.NewLsTool(root))
	registry.Register(tool.NewReadTool(root, 128*1024))
	registry.Register(tool.NewReminderDeleteTool(root))
	registry.Register(tool.NewReminderListTool(root))
	registry.Register(tool.NewReminderSetTool(root))
	registry.Register(tool.NewSecretDeleteTool(root))
	registry.Register(tool.NewSecretListTool(root))
	registry.Register(tool.NewSecretSetTool(root))
	registry.Register(tool.NewWebSearchTool(cfg.GeminiAPIKey, cfg.GeminiEndpoint))
	registry.Register(tool.NewWriteTool(root))
	return registry
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
	fmt.Fprintf(out, "provider: %s\n", cfg.Provider)
	fmt.Fprintf(out, "endpoint: %s\n", providerEndpoint(cfg))
	fmt.Fprintf(out, "log_file: %s\n", cfg.Log.File)
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
		if entry, ok, err := authstore.NewGlobalFileStore().Get(provider.OpenAICodex); err == nil && ok {
			status = authstore.FormatStatus(entry)
		}
		fmt.Fprintf(out, "openai_codex_auth: %s\n", status)
	default:
		fmt.Fprintln(out, "provider_auth: unknown")
	}
	fmt.Fprintf(out, "model: %s\n", cfg.ModelID)
	fmt.Fprintf(out, "session_dir: %s\n", cfg.Session.Dir)
	fmt.Fprintf(out, "skills_disabled: %t\n", cfg.Prompt.NoSkills)
	return cfg.Validate()
}

func runAgent(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cmd command.Run,
	cfg config.Runtime,
	newGateway func(cfg config.Runtime) model.Gateway,
	registry *tool.Registry,
	profile prompt.Profile,
) error {
	if cmd.Config.ServerURL != "" {
		_, err := httpapi.RenderAGUIRun(ctx, http.DefaultClient, cmd.Config.ServerURL, httpapi.RunRequest{
			Prompt:  cmd.Prompt,
			Session: sessionOptions(cmd.Session),
		}, out)
		if err != nil {
			return wrapError("model", err)
		}
		return nil
	}
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newGateway, registry, profile)
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
	newGateway func(cfg config.Runtime) model.Gateway,
	registry *tool.Registry,
	profile prompt.Profile,
) error {
	scanner := bufio.NewScanner(in)
	if cmd.Config.ServerURL != "" {
		return runRemoteChat(ctx, scanner, out, cmd)
	}
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newGateway, registry, profile)
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
		return wrapError("input", err)
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

func printUsage(out io.Writer) {
	fmt.Fprintln(out, brand.CLIName)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "usage:")
	fmt.Fprintf(out, "  %s help\n", brand.CLIName)
	fmt.Fprintf(out, "  %s doctor\n", brand.CLIName)
	fmt.Fprintf(out, "  %s run <prompt>\n", brand.CLIName)
	fmt.Fprintf(out, "  %s chat\n", brand.CLIName)
	fmt.Fprintf(out, "  %s serve\n", brand.CLIName)
	fmt.Fprintf(out, "  %s telegram [--poll-once]\n", brand.CLIName)
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
	fmt.Fprintln(out, "  --telegram-bot-token <token>")
	fmt.Fprintln(out, "  --telegram-endpoint <url>")
	fmt.Fprintln(out, "  --telegram-poll-timeout <duration>")
	fmt.Fprintln(out, "  --telegram-pairing-token <token>")
	fmt.Fprintln(out, "  --telegram-allow-user <id>")
	fmt.Fprintln(out, "  --heartbeat=<bool>")
	fmt.Fprintln(out, "  --heartbeat-interval <duration>")
	fmt.Fprintln(out, "  --log-file <path>")
	fmt.Fprintln(out, "  --log-level <level>")
	fmt.Fprintln(out, "  --server <url>")
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
			BotToken:     options.TelegramBotToken,
			Endpoint:     options.TelegramEndpoint,
			PollTimeout:  options.TelegramPollTimeout,
			PairingToken: options.TelegramPairingToken,
			AllowedUsers: append([]string(nil), options.TelegramAllowedUsers...),
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
	deleted, err := authstore.NewGlobalFileStore().Delete(kind)
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
	store := authstore.NewGlobalFileStore()
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
	if err := authstore.NewGlobalFileStore().PutOAuth(provider.OpenAICodex, creds); err != nil {
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

func newService(cwd string, cfg config.Runtime, newGateway func(cfg config.Runtime) model.Gateway, registry *tool.Registry, profile prompt.Profile) *agent.Service {
	return agent.NewServiceWithPromptProfile(cwd, cfg, newGateway, func(cfg config.Runtime) *tool.Registry {
		if registry != nil {
			return registry
		}
		return defaultToolRegistryForConfig(cfg)
	}, profile)
}

func validateProviderAccess(ctx context.Context, cwd string, cfg config.Runtime) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	_, err := auth.NewResolver(authstore.NewGlobalFileStore()).Resolve(ctx, cfg)
	return err
}

func configureLogger(cwd string, cfg config.Runtime) (func(), error) {
	path := strings.TrimSpace(cfg.Log.File)
	if path != "" && !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
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
	newGateway func(cfg config.Runtime) model.Gateway,
	registry *tool.Registry,
) error {
	_ = out
	if err := validateProviderAccess(ctx, cwd, cfg); err != nil {
		return wrapError("config", err)
	}
	if err := cfg.ValidateTelegram(); err != nil {
		return wrapError("config", err)
	}
	service := newService(cwd, cfg, newGateway, registry, prompt.ProfileGateway)
	gw := gatewayruntime.New(cwd, cfg, engine.WrapService(service))
	client := telegram.NewHTTPClient(http.DefaultClient, cfg.Telegram.Endpoint, cfg.Telegram.BotToken)
	adapter := telegram.NewAdapterWithPaths(gw.Paths(), client, gw, telegram.Config{
		PollTimeout:  cfg.Telegram.PollTimeout,
		PairingToken: cfg.Telegram.PairingToken,
		AllowedUsers: append([]string(nil), cfg.Telegram.AllowedUsers...),
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
	sessionReq := sessionOptions(cmd.Session)
	if cmd.Config.ServerURL == "" {
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
			summary, err := httpapi.RenderAGUIRun(ctx, http.DefaultClient, cmd.Config.ServerURL, httpapi.RunRequest{
				Prompt:  input.Text,
				Session: sessionReq,
			}, out)
			if err != nil {
				return err
			}
			if summary.SessionPath != "" {
				sessionReq = httpapi.SessionStartOptions{SessionPath: summary.SessionPath}
			}
		}
	}
	return scanner.Err()
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

func runServe(
	ctx context.Context,
	out io.Writer,
	cwd string,
	cmd command.Serve,
	cfg config.Runtime,
	newGateway func(cfg config.Runtime) model.Gateway,
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
	service := newService(cwd, cfg, newGateway, registry, profile)
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

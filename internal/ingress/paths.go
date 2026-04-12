package ingress

import (
	"path/filepath"
	"strings"

	"fritz/internal/config"
)

const CurrentStoreVersion = 1

type StatePaths struct {
	Root string

	MetaPath string

	RoutingDir            string
	RoutingSessionMapPath string

	TelegramDir           string
	TelegramOffsetPath    string
	TelegramAllowlistPath string
	TelegramPairingPath   string

	BindingsDir         string
	BindingsCurrentPath string

	HeartbeatDir       string
	HeartbeatStatePath string

	ReminderDir  string
	ReminderPath string

	legacyStatePath string
}

func ResolveStatePaths(cwd string, cfg config.Runtime) StatePaths {
	sessionDir := cfg.Session.Dir
	root := config.DefaultWorkspaceGatewayRoot(cwd)
	if strings.TrimSpace(sessionDir) != "" {
		if !filepath.IsAbs(sessionDir) {
			sessionDir = filepath.Join(cwd, sessionDir)
		}
		root = filepath.Join(filepath.Dir(filepath.Clean(sessionDir)), "gateway")
	}
	routingDir := filepath.Join(root, "routing")
	telegramDir := filepath.Join(root, "telegram")
	bindingsDir := filepath.Join(root, "bindings")
	heartbeatDir := filepath.Join(root, "heartbeat")
	reminderDir := filepath.Join(root, "reminders")
	return StatePaths{
		Root: root,

		MetaPath: filepath.Join(root, "meta.json"),

		RoutingDir:            routingDir,
		RoutingSessionMapPath: filepath.Join(routingDir, "session-map.json"),

		TelegramDir:           telegramDir,
		TelegramOffsetPath:    filepath.Join(telegramDir, "offset.json"),
		TelegramAllowlistPath: filepath.Join(telegramDir, "allowlist.json"),
		TelegramPairingPath:   filepath.Join(telegramDir, "pairing.json"),

		BindingsDir:         bindingsDir,
		BindingsCurrentPath: filepath.Join(bindingsDir, "current.json"),

		HeartbeatDir:       heartbeatDir,
		HeartbeatStatePath: filepath.Join(heartbeatDir, "state.json"),

		ReminderDir:  reminderDir,
		ReminderPath: filepath.Join(reminderDir, "reminders.json"),

		legacyStatePath: filepath.Join(root, "state.json"),
	}
}

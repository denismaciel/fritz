package telegram

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fritz/internal/ingress"
	"fritz/internal/logx"
	"fritz/internal/transcription"
)

type Client interface {
	GetUpdates(context.Context, GetUpdatesRequest) ([]Update, error)
	SendMessage(context.Context, SendMessageRequest) error
	GetFile(context.Context, string) (File, error)
	DownloadFile(context.Context, string) ([]byte, error)
}

type Handler interface {
	HandleInbound(context.Context, ingress.InboundMessage) (ingress.HandleResult, error)
}

type Adapter struct {
	paths   ingress.StatePaths
	client  Client
	handler Handler
	cfg     Config
}

type Config struct {
	PollTimeout  time.Duration
	PairingToken string
	AllowedUsers []string
	Transcriber  transcription.Service
}

func NewAdapter(stateRoot string, client Client, handler Handler, cfg Config) *Adapter {
	root := filepath.Dir(stateRoot)
	return NewAdapterWithPaths(ingress.StatePaths{
		Root:                  root,
		MetaPath:              filepath.Join(root, "meta.json"),
		TelegramDir:           stateRoot,
		TelegramOffsetPath:    filepath.Join(stateRoot, "offset.json"),
		TelegramAllowlistPath: filepath.Join(stateRoot, "allowlist.json"),
		TelegramPairingPath:   filepath.Join(stateRoot, "pairing.json"),
		BindingsDir:           filepath.Join(root, "bindings"),
		BindingsCurrentPath:   filepath.Join(root, "bindings", "current.json"),
	}, client, handler, cfg)
}

func NewAdapterWithPaths(paths ingress.StatePaths, client Client, handler Handler, cfg Config) *Adapter {
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 20 * time.Second
	}
	return &Adapter{
		paths:   paths,
		client:  client,
		handler: handler,
		cfg:     cfg,
	}
}

func (a *Adapter) PollOnce(ctx context.Context) (int, error) {
	logger := logx.Component("telegram").With().Str("event", "telegram.poll.once").Logger()
	if err := ingress.EnsureLayout(a.paths, time.Now().UTC()); err != nil {
		logger.Error().Err(err).Str("stage", "layout").Msg("")
		return 0, err
	}
	offset, err := a.loadOffset()
	if err != nil {
		logger.Error().Err(err).Str("stage", "offset.load").Msg("")
		return 0, err
	}
	allowedUsers, err := a.loadAllowedUsers()
	if err != nil {
		logger.Error().Err(err).Str("stage", "allowlist.load").Msg("")
		return 0, err
	}
	pairings, err := a.loadPairings()
	if err != nil {
		logger.Error().Err(err).Str("stage", "pairings.load").Msg("")
		return 0, err
	}
	updates, err := a.client.GetUpdates(ctx, GetUpdatesRequest{
		Offset:         offset,
		TimeoutSeconds: int(a.cfg.PollTimeout / time.Second),
	})
	if err != nil {
		logger.Error().Err(err).Str("stage", "get_updates").Int64("offset", offset).Msg("")
		return 0, err
	}
	logger.Info().Int64("offset", offset).Int("updates", len(updates)).Msg("")
	nextOffset := offset
	processed := 0
	for _, update := range updates {
		updateLogger := logger.With().Int64("update_id", update.UpdateID).Logger()
		if update.UpdateID >= nextOffset {
			nextOffset = update.UpdateID + 1
		}
		message, ok := a.normalizeUpdate(ctx, update)
		if !ok {
			updateLogger.Debug().Str("event", "telegram.update.skipped").Msg("")
			continue
		}
		allowed, reply, changed := a.authorize(message, allowedUsers, &pairings)
		if changed {
			if err := a.saveAllowedUsers(allowedUsers); err != nil {
				updateLogger.Error().Err(err).Str("stage", "allowlist.save").Msg("")
				return processed, err
			}
			if err := a.savePairings(pairings); err != nil {
				updateLogger.Error().Err(err).Str("stage", "pairings.save").Msg("")
				return processed, err
			}
		}
		if !allowed {
			updateLogger.Warn().Str("event", "telegram.auth.denied").Str("chat_id", message.ChatID).Str("user_id", message.UserID).Msg("")
			if strings.TrimSpace(reply) != "" {
				if err := a.sendReply(ctx, message.ChatID, reply); err != nil {
					updateLogger.Error().Err(err).Str("stage", "reply.unauthorized").Msg("")
					return processed, err
				}
			}
			processed++
			continue
		}
		if strings.TrimSpace(reply) != "" {
			updateLogger.Info().Str("event", "telegram.auth.paired").Str("chat_id", message.ChatID).Str("user_id", message.UserID).Msg("")
			if err := a.sendReply(ctx, message.ChatID, reply); err != nil {
				updateLogger.Error().Err(err).Str("stage", "reply.paired").Msg("")
				return processed, err
			}
			processed++
			continue
		}
		result, err := a.handler.HandleInbound(ctx, message)
		if err != nil {
			updateLogger.Error().Err(err).Str("stage", "ingress.handle").Msg("")
			return processed, err
		}
		updateLogger.Info().
			Str("event", "telegram.update.handled").
			Str("session_key", result.SessionKey).
			Int("messages", len(result.Messages)).
			Msg("")
		for _, outbound := range result.Messages {
			if strings.TrimSpace(outbound.Text) == "" {
				continue
			}
			if err := a.sendReply(ctx, outbound.ChatID, outbound.Text); err != nil {
				updateLogger.Error().Err(err).Str("stage", "reply.send").Msg("")
				return processed, err
			}
		}
		processed++
	}
	if nextOffset != offset {
		if err := a.saveOffset(nextOffset); err != nil {
			logger.Error().Err(err).Str("stage", "offset.save").Int64("next_offset", nextOffset).Msg("")
			return processed, err
		}
	}
	logger.Info().Str("event", "telegram.poll.done").Int("processed", processed).Int64("next_offset", nextOffset).Msg("")
	return processed, nil
}

func (a *Adapter) Run(ctx context.Context) error {
	logger := logx.Component("telegram")
	logger.Info().Str("event", "telegram.run.start").Msg("")
	for {
		if ctx.Err() != nil {
			return nil
		}
		if _, err := a.PollOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Error().Err(err).Str("event", "telegram.run.error").Msg("")
			return err
		}
	}
}

func (a *Adapter) authorize(
	message ingress.InboundMessage,
	allowedUsers map[string]struct{},
	pairings *ingress.TelegramPairingFile,
) (bool, string, bool) {
	userID := strings.TrimSpace(message.UserID)
	if _, ok := allowedUsers[userID]; ok && userID != "" {
		return true, "", false
	}
	if userID != "" && a.isPairingCommand(message) {
		allowedUsers[userID] = struct{}{}
		pairings.Paired = append(pairings.Paired, ingress.TelegramPairingRecord{
			UserID:   userID,
			ChatID:   strings.TrimSpace(message.ChatID),
			PairedAt: time.Now().UTC().Format(time.RFC3339Nano),
		})
		return false, "paired", true
	}
	if message.ChatType == ingress.ChatTypeDM {
		return false, "not authorized", false
	}
	return false, "", false
}

func (a *Adapter) isPairingCommand(message ingress.InboundMessage) bool {
	if message.ChatType != ingress.ChatTypeDM {
		return false
	}
	token := strings.TrimSpace(a.cfg.PairingToken)
	if token == "" {
		return false
	}
	text := strings.Fields(strings.TrimSpace(message.Text))
	if len(text) != 2 {
		return false
	}
	return (text[0] == "/start" || text[0] == "/pair") && text[1] == token
}

func NormalizeUpdate(update Update) (ingress.InboundMessage, bool) {
	return normalizeUpdate(update, "")
}

func normalizeUpdate(update Update, transcript string) (ingress.InboundMessage, bool) {
	if update.Message == nil {
		return ingress.InboundMessage{}, false
	}
	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		text = strings.TrimSpace(update.Message.Caption)
	}
	audio, hasAudio := selectAudio(update.Message)
	body := text
	if strings.TrimSpace(transcript) != "" && hasAudio {
		if text != "" {
			body = "[Audio]\nUser text:\n" + text + "\nTranscript:\n" + strings.TrimSpace(transcript)
		} else {
			body = "[Audio]\nTranscript:\n" + strings.TrimSpace(transcript)
		}
	}
	if strings.TrimSpace(body) == "" {
		return ingress.InboundMessage{}, false
	}
	chatType := ingress.ChatTypeGroup
	if update.Message.Chat.Type == "private" {
		chatType = ingress.ChatTypeDM
	}
	message := ingress.InboundMessage{
		Channel:  "telegram",
		ChatType: chatType,
		ChatID:   strconv.FormatInt(update.Message.Chat.ID, 10),
		Text:     body,
		Metadata: map[string]string{},
	}
	if update.Message.From != nil {
		message.UserID = strconv.FormatInt(update.Message.From.ID, 10)
		if update.Message.From.Username != "" {
			message.Metadata["from_username"] = update.Message.From.Username
		}
	}
	if update.Message.Chat.Title != "" {
		message.Metadata["chat_title"] = update.Message.Chat.Title
	}
	if update.Message.Document != nil {
		if update.Message.Document.FileName != "" {
			message.Metadata["document_name"] = update.Message.Document.FileName
		}
		if update.Message.Document.MimeType != "" {
			message.Metadata["document_mime"] = update.Message.Document.MimeType
		}
	}
	if len(update.Message.Photo) > 0 {
		message.Metadata["photo_count"] = strconv.Itoa(len(update.Message.Photo))
	}
	if hasAudio {
		if audio.MIMEType != "" {
			message.Metadata["audio_mime"] = audio.MIMEType
		}
		if audio.FileName != "" {
			message.Metadata["audio_name"] = audio.FileName
		}
		if audio.Duration > 0 {
			message.Metadata["voice_duration"] = strconv.Itoa(audio.Duration)
		}
	}
	return message, true
}

func (a *Adapter) normalizeUpdate(ctx context.Context, update Update) (ingress.InboundMessage, bool) {
	message, ok := NormalizeUpdate(update)
	if ok {
		if !hasAudioAttachment(update.Message) {
			return message, true
		}
		if a.cfg.Transcriber == nil {
			return message, true
		}
	}
	if update.Message == nil || a.cfg.Transcriber == nil || !hasAudioAttachment(update.Message) {
		return message, ok
	}
	audio, found := selectAudio(update.Message)
	if !found || strings.TrimSpace(audio.FileID) == "" {
		return message, ok
	}
	file, err := a.client.GetFile(ctx, audio.FileID)
	if err != nil || strings.TrimSpace(file.FilePath) == "" {
		return message, ok
	}
	body, err := a.client.DownloadFile(ctx, file.FilePath)
	if err != nil || len(body) == 0 {
		return message, ok
	}
	transcript, err := a.cfg.Transcriber.Transcribe(ctx, transcription.AudioInput{
		Bytes:    body,
		MIMEType: audio.MIMEType,
		FileName: audio.FileName,
	})
	if err != nil || strings.TrimSpace(transcript) == "" {
		return message, ok
	}
	return normalizeUpdate(update, transcript)
}

type audioAttachment struct {
	FileID   string
	FileName string
	MIMEType string
	Duration int
}

func hasAudioAttachment(message *Message) bool {
	_, ok := selectAudio(message)
	return ok
}

func selectAudio(message *Message) (audioAttachment, bool) {
	if message == nil {
		return audioAttachment{}, false
	}
	if message.Voice != nil && strings.TrimSpace(message.Voice.FileID) != "" {
		return audioAttachment{
			FileID:   strings.TrimSpace(message.Voice.FileID),
			MIMEType: strings.TrimSpace(message.Voice.MimeType),
			Duration: message.Voice.Duration,
			FileName: "voice.ogg",
		}, true
	}
	if message.Audio != nil && strings.TrimSpace(message.Audio.FileID) != "" {
		return audioAttachment{
			FileID:   strings.TrimSpace(message.Audio.FileID),
			FileName: strings.TrimSpace(message.Audio.FileName),
			MIMEType: strings.TrimSpace(message.Audio.MimeType),
			Duration: message.Audio.Duration,
		}, true
	}
	if message.Document != nil && strings.HasPrefix(strings.TrimSpace(message.Document.MimeType), "audio/") {
		return audioAttachment{
			FileID:   strings.TrimSpace(message.Document.FileID),
			FileName: strings.TrimSpace(message.Document.FileName),
			MIMEType: strings.TrimSpace(message.Document.MimeType),
		}, true
	}
	return audioAttachment{}, false
}

func (a *Adapter) loadOffset() (int64, error) {
	state, exists, err := ingress.ReadJSONFile(a.paths.TelegramOffsetPath, ingress.TelegramOffsetFile{})
	if err != nil {
		return 0, err
	}
	if exists {
		if state.Version == 0 {
			state.Version = ingress.CurrentStoreVersion
			if err := ingress.WriteJSONFileAtomic(a.paths.TelegramOffsetPath, state); err != nil {
				return 0, err
			}
		}
		return state.NextOffset, nil
	}
	var legacy struct {
		NextOffset int64 `json:"nextOffset"`
	}
	legacy, exists, err = ingress.ReadJSONFile(a.paths.TelegramOffsetPath, legacy)
	if err != nil || !exists {
		return 0, err
	}
	state = ingress.TelegramOffsetFile{Version: ingress.CurrentStoreVersion, NextOffset: legacy.NextOffset}
	if err := ingress.WriteJSONFileAtomic(a.paths.TelegramOffsetPath, state); err != nil {
		return 0, err
	}
	return state.NextOffset, nil
}

func (a *Adapter) loadAllowedUsers() (map[string]struct{}, error) {
	allowed := map[string]struct{}{}
	for _, userID := range a.cfg.AllowedUsers {
		userID = strings.TrimSpace(userID)
		if userID != "" {
			allowed[userID] = struct{}{}
		}
	}
	state, exists, err := ingress.ReadJSONFile(a.paths.TelegramAllowlistPath, ingress.TelegramAllowlistFile{})
	if err != nil {
		return nil, err
	}
	if !exists {
		var legacy struct {
			Users []string `json:"users"`
		}
		legacy, exists, err = ingress.ReadJSONFile(a.paths.TelegramAllowlistPath, legacy)
		if err != nil {
			return nil, err
		}
		if exists {
			state = ingress.TelegramAllowlistFile{Version: ingress.CurrentStoreVersion, Users: legacy.Users}
			if err := ingress.WriteJSONFileAtomic(a.paths.TelegramAllowlistPath, state); err != nil {
				return nil, err
			}
		}
	}
	if state.Version == 0 && exists {
		state.Version = ingress.CurrentStoreVersion
		if err := ingress.WriteJSONFileAtomic(a.paths.TelegramAllowlistPath, state); err != nil {
			return nil, err
		}
	}
	for _, userID := range state.Users {
		userID = strings.TrimSpace(userID)
		if userID != "" {
			allowed[userID] = struct{}{}
		}
	}
	return allowed, nil
}

func (a *Adapter) saveOffset(nextOffset int64) error {
	return ingress.WriteJSONFileAtomic(a.paths.TelegramOffsetPath, ingress.TelegramOffsetFile{
		Version:    ingress.CurrentStoreVersion,
		NextOffset: nextOffset,
	})
}

func (a *Adapter) saveAllowedUsers(users map[string]struct{}) error {
	list := make([]string, 0, len(users))
	for userID := range users {
		if strings.TrimSpace(userID) != "" {
			list = append(list, userID)
		}
	}
	sort.Strings(list)
	return ingress.WriteJSONFileAtomic(a.paths.TelegramAllowlistPath, ingress.TelegramAllowlistFile{
		Version: ingress.CurrentStoreVersion,
		Users:   list,
	})
}

func (a *Adapter) loadPairings() (ingress.TelegramPairingFile, error) {
	state, exists, err := ingress.ReadJSONFile(a.paths.TelegramPairingPath, ingress.TelegramPairingFile{})
	if err != nil {
		return ingress.TelegramPairingFile{}, err
	}
	if !exists {
		return ingress.TelegramPairingFile{Version: ingress.CurrentStoreVersion, Paired: []ingress.TelegramPairingRecord{}}, nil
	}
	if state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
	}
	if state.Paired == nil {
		state.Paired = []ingress.TelegramPairingRecord{}
	}
	return state, nil
}

func (a *Adapter) savePairings(state ingress.TelegramPairingFile) error {
	if state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
	}
	if state.Paired == nil {
		state.Paired = []ingress.TelegramPairingRecord{}
	}
	return ingress.WriteJSONFileAtomic(a.paths.TelegramPairingPath, state)
}

func (a *Adapter) sendReply(ctx context.Context, chatID string, text string) error {
	parsed, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat id %q", chatID)
	}
	return a.client.SendMessage(ctx, SendMessageRequest{
		ChatID: parsed,
		Text:   text,
	})
}

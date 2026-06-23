package telegram

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fritz/internal/ingress"
	"fritz/internal/logx"
	"fritz/internal/tool"
	"fritz/internal/transcription"
)

const maxGroupContextMessages = 80
const fallbackBotUsername = "fritz"
const telegramMaxMessageRunes = 4096
const telegramReplyChunkRunes = 3000
const telegramPlainChunkRunes = 3900

type Client interface {
	GetMe(context.Context) (BotInfo, error)
	GetUpdates(context.Context, GetUpdatesRequest) ([]Update, error)
	SendMessage(context.Context, SendMessageRequest) error
	GetFile(context.Context, string) (File, error)
	DownloadFile(context.Context, string) ([]byte, error)
}

type Handler interface {
	HandleInbound(context.Context, ingress.InboundMessage) (ingress.HandleResult, error)
}

type Adapter struct {
	paths       ingress.StatePaths
	client      Client
	handler     Handler
	cfg         Config
	botUsername string
}

type Config struct {
	PollTimeout  time.Duration
	PairingToken string
	AllowedUsers []string
	Transcriber  transcription.Service
}

type groupContextFile struct {
	Version  int                   `json:"version"`
	ChatID   string                `json:"chatId"`
	Messages []groupContextMessage `json:"messages"`
}

type groupContextMessage struct {
	MessageID string `json:"messageId,omitempty"`
	UserID    string `json:"userId,omitempty"`
	Username  string `json:"username,omitempty"`
	SentAt    string `json:"sentAt,omitempty"`
	Text      string `json:"text"`
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
		AllowedUpdates: []string{"message"},
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
			if message.ChatType == ingress.ChatTypeGroup {
				if err := a.appendGroupContext(message); err != nil {
					updateLogger.Error().Err(err).Str("stage", "group_context.save").Msg("")
					return processed, err
				}
				updateLogger.Debug().Str("event", "telegram.group.context_only").Str("chat_id", message.ChatID).Msg("")
				processed++
				continue
			}
			if strings.TrimSpace(reply) != "" {
				if err := a.sendReply(ctx, message.ChatID, reply); err != nil {
					updateLogger.Error().Err(err).Str("stage", "reply.unauthorized").Msg("")
					return processed, err
				}
			}
			processed++
			continue
		}
		if message.ChatType == ingress.ChatTypeGroup {
			originalMessage := message
			addressed := a.isAddressedGroupMessage(ctx, message)
			if !addressed {
				if err := a.appendGroupContext(message); err != nil {
					updateLogger.Error().Err(err).Str("stage", "group_context.save").Msg("")
					return processed, err
				}
				updateLogger.Debug().Str("event", "telegram.group.context_only").Str("chat_id", message.ChatID).Msg("")
				processed++
				continue
			}
			contextMessages, err := a.loadGroupContext(message.ChatID)
			if err != nil {
				updateLogger.Error().Err(err).Str("stage", "group_context.load").Msg("")
				return processed, err
			}
			message.Text = buildGroupPrompt(contextMessages.Messages, message)
			if err := a.appendGroupContext(originalMessage); err != nil {
				updateLogger.Error().Err(err).Str("stage", "group_context.save").Msg("")
				return processed, err
			}
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
			if replyErr := a.sendReply(ctx, message.ChatID, telegramErrorReply(err)); replyErr != nil {
				updateLogger.Error().Err(replyErr).Str("stage", "reply.error").Msg("")
				return processed, replyErr
			}
			processed++
			continue
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
			if message.ChatType == ingress.ChatTypeGroup {
				_ = a.appendGroupContext(ingress.InboundMessage{
					ChatType: message.ChatType,
					ChatID:   outbound.ChatID,
					UserID:   "fritz",
					Text:     outbound.Text,
					Metadata: map[string]string{
						"from_username": "fritz",
						"sent_at":       time.Now().UTC().Format(time.RFC3339),
					},
				})
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
	if body == "" && len(update.Message.Photo) > 0 {
		body = "[Image]"
	}
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
	if update.Message.MessageID != 0 {
		message.Metadata["message_id"] = strconv.FormatInt(update.Message.MessageID, 10)
	}
	if update.Message.Date > 0 {
		message.Metadata["sent_at"] = time.Unix(update.Message.Date, 0).UTC().Format(time.RFC3339)
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
	if ok && len(update.Message.Photo) > 0 {
		message.Images = a.downloadPhotos(ctx, update.Message)
		if len(message.Images) > 0 {
			message.Metadata["image_count"] = strconv.Itoa(len(message.Images))
		}
	}
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

func (a *Adapter) downloadPhotos(ctx context.Context, message *Message) []tool.ContentPart {
	photo, ok := selectPhoto(message)
	if !ok {
		return nil
	}
	file, err := a.client.GetFile(ctx, photo.FileID)
	if err != nil || strings.TrimSpace(file.FilePath) == "" {
		return nil
	}
	body, err := a.client.DownloadFile(ctx, file.FilePath)
	if err != nil || len(body) == 0 {
		return nil
	}
	return []tool.ContentPart{tool.ImagePart(mimeTypeForTelegramPath(file.FilePath), base64.StdEncoding.EncodeToString(body))}
}

func selectPhoto(message *Message) (Photo, bool) {
	if message == nil || len(message.Photo) == 0 {
		return Photo{}, false
	}
	for i := len(message.Photo) - 1; i >= 0; i-- {
		if strings.TrimSpace(message.Photo[i].FileID) != "" {
			return Photo{FileID: strings.TrimSpace(message.Photo[i].FileID)}, true
		}
	}
	return Photo{}, false
}

func mimeTypeForTelegramPath(path string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
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

func (a *Adapter) appendGroupContext(message ingress.InboundMessage) error {
	chatID := strings.TrimSpace(message.ChatID)
	if chatID == "" || strings.TrimSpace(message.Text) == "" {
		return nil
	}
	state, err := a.loadGroupContext(chatID)
	if err != nil {
		return err
	}
	state.Messages = append(state.Messages, groupContextMessage{
		MessageID: strings.TrimSpace(message.Metadata["message_id"]),
		UserID:    strings.TrimSpace(message.UserID),
		Username:  strings.TrimSpace(message.Metadata["from_username"]),
		SentAt:    firstNonEmpty(strings.TrimSpace(message.Metadata["sent_at"]), time.Now().UTC().Format(time.RFC3339)),
		Text:      strings.TrimSpace(message.Text),
	})
	if len(state.Messages) > maxGroupContextMessages {
		state.Messages = state.Messages[len(state.Messages)-maxGroupContextMessages:]
	}
	return ingress.WriteJSONFileAtomic(a.groupContextPath(chatID), state)
}

func (a *Adapter) loadGroupContext(chatID string) (groupContextFile, error) {
	chatID = strings.TrimSpace(chatID)
	state, exists, err := ingress.ReadJSONFile(a.groupContextPath(chatID), groupContextFile{})
	if err != nil {
		return groupContextFile{}, err
	}
	if !exists {
		return groupContextFile{Version: ingress.CurrentStoreVersion, ChatID: chatID}, nil
	}
	if state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
	}
	if state.ChatID == "" {
		state.ChatID = chatID
	}
	return state, nil
}

func (a *Adapter) groupContextPath(chatID string) string {
	name := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(strings.TrimSpace(chatID))
	if name == "" {
		name = "unknown"
	}
	return filepath.Join(a.paths.TelegramDir, "groups", name+".json")
}

func (a *Adapter) isAddressedGroupMessage(ctx context.Context, message ingress.InboundMessage) bool {
	text := strings.TrimSpace(strings.ToLower(message.Text))
	if text == "" {
		return false
	}
	names := a.botAddressNames(ctx)
	fields := strings.Fields(text)
	if len(fields) > 0 {
		first := strings.TrimRight(fields[0], ":,")
		if first == "/fritz" {
			return true
		}
		for _, name := range names {
			if first == "@"+name || first == "/fritz@"+name {
				return true
			}
		}
	}
	for _, name := range names {
		if strings.Contains(text, "@"+name+" ") || strings.HasSuffix(text, "@"+name) {
			return true
		}
	}
	return false
}

func (a *Adapter) botAddressNames(ctx context.Context) []string {
	names := map[string]struct{}{
		fallbackBotUsername: {},
		"fritz_bot":         {},
	}
	if username := strings.TrimSpace(a.botUsername); username != "" {
		names[strings.ToLower(strings.TrimPrefix(username, "@"))] = struct{}{}
	} else if info, err := a.client.GetMe(ctx); err == nil {
		a.botUsername = strings.TrimSpace(info.Username)
		if a.botUsername != "" {
			names[strings.ToLower(strings.TrimPrefix(a.botUsername, "@"))] = struct{}{}
		}
	}
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func buildGroupPrompt(messages []groupContextMessage, current ingress.InboundMessage) string {
	var builder strings.Builder
	builder.WriteString("Telegram group context. Each line is [time] sender: message.\n")
	for _, message := range messages {
		text := strings.TrimSpace(message.Text)
		if text == "" {
			continue
		}
		builder.WriteString("[")
		builder.WriteString(firstNonEmpty(message.SentAt, "unknown time"))
		builder.WriteString("] ")
		builder.WriteString(groupSenderLabel(message))
		builder.WriteString(": ")
		builder.WriteString(sanitizeTelegramPromptText(text))
		builder.WriteString("\n")
	}
	builder.WriteString("\nAddressed request:\n")
	builder.WriteString(groupSenderLabel(groupContextMessage{
		UserID:   current.UserID,
		Username: current.Metadata["from_username"],
	}))
	builder.WriteString(": ")
	builder.WriteString(sanitizeTelegramPromptText(stripBotAddress(strings.TrimSpace(current.Text))))
	return builder.String()
}

func groupSenderLabel(message groupContextMessage) string {
	switch {
	case strings.TrimSpace(message.Username) != "" && strings.TrimSpace(message.UserID) != "":
		return "user " + strings.TrimPrefix(strings.TrimSpace(message.Username), "@") + " (id " + strings.TrimSpace(message.UserID) + ")"
	case strings.TrimSpace(message.Username) != "":
		return "user " + strings.TrimPrefix(strings.TrimSpace(message.Username), "@")
	case strings.TrimSpace(message.UserID) != "":
		return "id " + strings.TrimSpace(message.UserID)
	default:
		return "unknown"
	}
}

func sanitizeTelegramPromptText(text string) string {
	return strings.ReplaceAll(strings.TrimSpace(text), "@", "at ")
}

func stripBotAddress(text string) string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return ""
	}
	first := strings.TrimRight(strings.ToLower(fields[0]), ":,")
	if first == "/fritz" || strings.HasPrefix(first, "/fritz@") || strings.HasPrefix(first, "@") {
		return strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return strings.TrimSpace(text)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (a *Adapter) sendReply(ctx context.Context, chatID string, text string) error {
	parsed, err := strconv.ParseInt(strings.TrimSpace(chatID), 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram chat id %q", chatID)
	}
	for _, chunk := range splitTelegramText(text, telegramReplyChunkRunes) {
		rendered := markdownToTelegramHTML(chunk)
		if len([]rune(rendered)) <= telegramMaxMessageRunes {
			if err := a.client.SendMessage(ctx, SendMessageRequest{
				ChatID:    parsed,
				Text:      rendered,
				ParseMode: "HTML",
			}); err != nil {
				return err
			}
			continue
		}
		for _, plain := range splitTelegramText(chunk, telegramPlainChunkRunes) {
			if err := a.client.SendMessage(ctx, SendMessageRequest{
				ChatID: parsed,
				Text:   plain,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func splitTelegramText(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= limit {
			chunks = append(chunks, strings.TrimSpace(string(runes)))
			break
		}
		cut := telegramChunkCut(runes, limit)
		chunks = append(chunks, strings.TrimSpace(string(runes[:cut])))
		runes = []rune(strings.TrimSpace(string(runes[cut:])))
	}
	return chunks
}

func telegramErrorReply(err error) string {
	text := "I hit an internal error while handling that message."
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		detail := strings.TrimSpace(err.Error())
		if len([]rune(detail)) > 1000 {
			detail = string([]rune(detail)[:1000]) + "..."
		}
		text += "\n\n" + detail
	}
	return text
}

func telegramChunkCut(runes []rune, limit int) int {
	cut := limit
	for i := limit; i > limit/2; i-- {
		if runes[i-1] == '\n' {
			return i
		}
	}
	for i := limit; i > limit/2; i-- {
		if runes[i-1] == ' ' {
			return i
		}
	}
	return cut
}

package slack

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fritz/internal/engine"
	"fritz/internal/ingress"
	"fritz/internal/logx"
)

const maxRecentEvents = 500

type Client interface {
	OpenSocketConnection(context.Context) (*SocketConn, error)
	PostMessage(context.Context, PostMessageRequest) error
	StartStream(context.Context, string, string) (StreamHandle, error)
	AppendStream(context.Context, StreamHandle, string) error
	StopStream(context.Context, StreamHandle) error
	SetStatus(context.Context, AssistantThreadRef, string) error
	SetTitle(context.Context, AssistantThreadRef, string) error
	SetSuggestedPrompts(context.Context, AssistantThreadRef, []SuggestedPrompt) error
	ConversationsReplies(context.Context, string, string) ([]HistoryMessage, error)
	UploadFile(context.Context, UploadFileRequest) error
}

type Handler interface {
	HandleInbound(context.Context, ingress.InboundMessage) (ingress.HandleResult, error)
	HandleInboundStream(context.Context, ingress.InboundMessage, func(engine.Event) error) (ingress.HandleResult, error)
	ClearSessionKey(context.Context, string) error
}

type Config struct {
	AllowedUsers    []string
	AllowedChannels []string
	Assistant       bool
}

type Adapter struct {
	paths   ingress.StatePaths
	client  Client
	handler Handler
	cfg     Config
}

func NewAdapterWithPaths(paths ingress.StatePaths, client Client, handler Handler, cfg Config) *Adapter {
	return &Adapter{
		paths:   paths,
		client:  client,
		handler: handler,
		cfg:     cfg,
	}
}

func (a *Adapter) Run(ctx context.Context) error {
	logger := logx.Component("slack")
	if err := ingress.EnsureLayout(a.paths, time.Now().UTC()); err != nil {
		return err
	}
	for {
		if ctx.Err() != nil {
			return nil
		}
		socket, err := a.client.OpenSocketConnection(ctx)
		if err != nil {
			logger.Error().Err(err).Str("event", "slack.socket.open").Msg("")
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(time.Second):
				continue
			}
		}
		err = a.consumeSocket(ctx, socket)
		_ = socket.Close()
		if ctx.Err() != nil {
			return nil
		}
		logger.Error().Err(err).Str("event", "slack.socket.consume").Msg("")
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (a *Adapter) consumeSocket(ctx context.Context, socket *SocketConn) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		envelope, err := socket.ReadEnvelope()
		if err != nil {
			return err
		}
		if strings.TrimSpace(envelope.EnvelopeID) != "" {
			if err := socket.Ack(envelope.EnvelopeID); err != nil {
				return err
			}
		}
		if envelope.Type != "events_api" {
			continue
		}
		var payload EventsAPIEnvelope
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			continue
		}
		if a.isSeenEvent(payload.EventID) {
			continue
		}
		if err := a.handleEventsAPI(ctx, payload); err != nil {
			logger := logx.Component("slack")
			logger.Error().Err(err).Str("event_id", payload.EventID).Msg("")
			continue
		}
		_ = a.recordSeenEvent(payload.EventID)
	}
}

func (a *Adapter) handleEventsAPI(ctx context.Context, payload EventsAPIEnvelope) error {
	var event Event
	if err := json.Unmarshal(payload.Event, &event); err != nil {
		return err
	}
	raw := map[string]any{}
	_ = json.Unmarshal(payload.Event, &raw)

	switch event.Type {
	case "assistant_thread_started":
		return a.handleAssistantThreadStarted(ctx, payload.TeamID, raw)
	case "assistant_thread_context_changed":
		return a.handleAssistantThreadContextChanged(ctx, payload.TeamID, raw)
	case "app_mention", "message":
		return a.handleMessageEvent(ctx, payload.TeamID, payload.EventID, event, raw)
	default:
		return nil
	}
}

func (a *Adapter) handleAssistantThreadStarted(ctx context.Context, teamID string, raw map[string]any) error {
	if !a.cfg.Assistant {
		return nil
	}
	ref, routeKey, binding := a.bindingFromAssistantEvent(teamID, raw)
	if routeKey == "" {
		return nil
	}
	if err := a.upsertContext(binding); err != nil {
		return err
	}
	return a.client.SetSuggestedPrompts(ctx, ref, defaultSuggestedPrompts())
}

func (a *Adapter) handleAssistantThreadContextChanged(_ context.Context, teamID string, raw map[string]any) error {
	_, routeKey, binding := a.bindingFromAssistantEvent(teamID, raw)
	if routeKey == "" {
		return nil
	}
	return a.upsertContext(binding)
}

func (a *Adapter) handleMessageEvent(ctx context.Context, teamID string, eventID string, event Event, raw map[string]any) error {
	if event.Subtype != "" || strings.TrimSpace(event.BotID) != "" {
		return nil
	}
	chatType := chatTypeForEvent(event)
	rootTS := threadRootTS(event)
	assistantBinding := a.lookupContext(teamID, event.Channel, rootTS)
	assistant := assistantBinding.RouteKey != ""
	if event.Type == "message" && chatType != ingress.ChatTypeDM && !assistant {
		return nil
	}
	if !a.allowed(event.User, event.Channel) {
		return a.client.PostMessage(ctx, PostMessageRequest{
			Channel:  event.Channel,
			ThreadTS: threadRootTS(event),
			Text:     "not authorized",
		})
	}

	text := normalizeEventText(event)
	if text == "" {
		return nil
	}
	sessionKey := buildRouteKey(teamID, event.Channel, rootTS, chatType, assistant)
	if strings.EqualFold(strings.TrimSpace(text), "/clear") {
		if err := a.handler.ClearSessionKey(ctx, sessionKey); err != nil {
			return err
		}
		if assistantBinding.RouteKey != "" {
			_ = a.deleteContext(assistantBinding.RouteKey)
		}
		return a.client.PostMessage(ctx, PostMessageRequest{
			Channel:  event.Channel,
			ThreadTS: rootTS,
			Text:     "history cleared",
		})
	}

	threadContext := ""
	if rootTS != "" {
		threadContext, _ = a.buildThreadContext(ctx, event.Channel, rootTS)
	}
	artifactDir := filepath.Join(a.paths.SlackDir, "artifacts", sanitizePathPart(eventID))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	prompt := decoratePrompt(text, threadContext, artifactDir, assistantBinding)
	inbound := ingress.InboundMessage{
		Channel:    "slack",
		ChatType:   chatType,
		ChatID:     event.Channel,
		UserID:     event.User,
		SessionKey: sessionKey,
		Text:       prompt,
		Metadata: map[string]string{
			"team_id":    teamID,
			"channel_id": event.Channel,
			"thread_ts":  rootTS,
			"event_id":   eventID,
		},
	}

	ref := AssistantThreadRef{}
	if assistant {
		ref = AssistantThreadRef{ChannelID: event.Channel, ThreadTS: rootTS}
		_ = a.client.SetStatus(ctx, ref, "Thinking...")
		defer func() {
			_ = a.client.SetStatus(context.Background(), ref, "")
		}()
	}
	renderer := newFinalRenderer(a.client, event.Channel, rootTS)
	result, err := a.handler.HandleInboundStream(ctx, inbound, renderer.Emit)
	if err != nil {
		return err
	}
	if err := renderer.Finish(ctx, result); err != nil {
		return err
	}
	if assistant && strings.TrimSpace(assistantBinding.Title) == "" {
		title := summarizeTitle(text)
		if title != "" {
			_ = a.client.SetTitle(ctx, ref, title)
			assistantBinding.Title = title
			assistantBinding.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			_ = a.upsertContext(assistantBinding)
		}
	}
	return a.uploadArtifacts(ctx, artifactDir, event.Channel, rootTS)
}

func (a *Adapter) allowed(userID string, channelID string) bool {
	users, channels, err := a.loadAllowlist()
	if err != nil {
		return true
	}
	if len(users) == 0 && len(channels) == 0 {
		return true
	}
	if _, ok := users[strings.TrimSpace(userID)]; ok {
		return true
	}
	if _, ok := channels[strings.TrimSpace(channelID)]; ok {
		return true
	}
	return false
}

func (a *Adapter) loadAllowlist() (map[string]struct{}, map[string]struct{}, error) {
	users := map[string]struct{}{}
	channels := map[string]struct{}{}
	for _, value := range a.cfg.AllowedUsers {
		value = strings.TrimSpace(value)
		if value != "" {
			users[value] = struct{}{}
		}
	}
	for _, value := range a.cfg.AllowedChannels {
		value = strings.TrimSpace(value)
		if value != "" {
			channels[value] = struct{}{}
		}
	}
	state, exists, err := ingress.ReadJSONFile(a.paths.SlackAllowlistPath, ingress.SlackAllowlistFile{})
	if err != nil {
		return nil, nil, err
	}
	if exists && state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
		_ = ingress.WriteJSONFileAtomic(a.paths.SlackAllowlistPath, state)
	}
	for _, value := range state.Users {
		value = strings.TrimSpace(value)
		if value != "" {
			users[value] = struct{}{}
		}
	}
	for _, value := range state.Channels {
		value = strings.TrimSpace(value)
		if value != "" {
			channels[value] = struct{}{}
		}
	}
	return users, channels, nil
}

func (a *Adapter) isSeenEvent(eventID string) bool {
	state, _, err := ingress.ReadJSONFile(a.paths.SlackEventStatePath, ingress.SlackEventStateFile{})
	if err != nil {
		return false
	}
	for _, candidate := range state.RecentEvents {
		if candidate == eventID {
			return true
		}
	}
	return false
}

func (a *Adapter) recordSeenEvent(eventID string) error {
	if strings.TrimSpace(eventID) == "" {
		return nil
	}
	state, _, err := ingress.ReadJSONFile(a.paths.SlackEventStatePath, ingress.SlackEventStateFile{})
	if err != nil {
		return err
	}
	if state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
	}
	state.RecentEvents = append(state.RecentEvents, eventID)
	if len(state.RecentEvents) > maxRecentEvents {
		state.RecentEvents = append([]string(nil), state.RecentEvents[len(state.RecentEvents)-maxRecentEvents:]...)
	}
	return ingress.WriteJSONFileAtomic(a.paths.SlackEventStatePath, state)
}

func (a *Adapter) loadContexts() (ingress.SlackContextStateFile, error) {
	state, _, err := ingress.ReadJSONFile(a.paths.SlackContextStatePath, ingress.SlackContextStateFile{})
	if err != nil {
		return ingress.SlackContextStateFile{}, err
	}
	if state.Version == 0 {
		state.Version = ingress.CurrentStoreVersion
	}
	if state.Bindings == nil {
		state.Bindings = []ingress.SlackContextBinding{}
	}
	return state, nil
}

func (a *Adapter) upsertContext(binding ingress.SlackContextBinding) error {
	if strings.TrimSpace(binding.RouteKey) == "" {
		return nil
	}
	state, err := a.loadContexts()
	if err != nil {
		return err
	}
	found := false
	for index := range state.Bindings {
		if state.Bindings[index].RouteKey == binding.RouteKey {
			state.Bindings[index] = binding
			found = true
			break
		}
	}
	if !found {
		state.Bindings = append(state.Bindings, binding)
	}
	sort.Slice(state.Bindings, func(i, j int) bool {
		return state.Bindings[i].RouteKey < state.Bindings[j].RouteKey
	})
	return ingress.WriteJSONFileAtomic(a.paths.SlackContextStatePath, state)
}

func (a *Adapter) deleteContext(routeKey string) error {
	state, err := a.loadContexts()
	if err != nil {
		return err
	}
	filtered := state.Bindings[:0]
	for _, binding := range state.Bindings {
		if binding.RouteKey == routeKey {
			continue
		}
		filtered = append(filtered, binding)
	}
	state.Bindings = filtered
	return ingress.WriteJSONFileAtomic(a.paths.SlackContextStatePath, state)
}

func (a *Adapter) lookupContext(teamID string, channelID string, threadTS string) ingress.SlackContextBinding {
	state, err := a.loadContexts()
	if err != nil {
		return ingress.SlackContextBinding{}
	}
	wantRoute := buildRouteKey(teamID, channelID, threadTS, ingress.ChatTypeDM, true)
	for _, binding := range state.Bindings {
		if binding.RouteKey == wantRoute {
			return binding
		}
	}
	if strings.TrimSpace(threadTS) == "" {
		var latest ingress.SlackContextBinding
		for _, binding := range state.Bindings {
			if binding.TeamID != teamID || binding.ChannelID != channelID {
				continue
			}
			if latest.UpdatedAt < binding.UpdatedAt {
				latest = binding
			}
		}
		return latest
	}
	return ingress.SlackContextBinding{}
}

func (a *Adapter) bindingFromAssistantEvent(teamID string, raw map[string]any) (AssistantThreadRef, string, ingress.SlackContextBinding) {
	channelID := stringField(raw, "channel")
	threadTS := stringField(raw, "thread_ts")
	contextMap := map[string]any{}
	if assistant, ok := raw["assistant_thread"].(map[string]any); ok {
		if channelID == "" {
			channelID = stringField(assistant, "channel_id")
		}
		if threadTS == "" {
			threadTS = stringField(assistant, "thread_ts")
		}
		if value, ok := assistant["context"].(map[string]any); ok {
			contextMap = value
		}
	}
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	metadata := map[string]string{}
	for key, value := range contextMap {
		switch typed := value.(type) {
		case string:
			metadata[key] = typed
		case float64:
			metadata[key] = fmt.Sprintf("%v", typed)
		}
	}
	routeKey := buildRouteKey(teamID, channelID, threadTS, ingress.ChatTypeDM, true)
	binding := ingress.SlackContextBinding{
		RouteKey:  routeKey,
		TeamID:    teamID,
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Metadata:  metadata,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	return AssistantThreadRef{ChannelID: channelID, ThreadTS: threadTS}, routeKey, binding
}

func (a *Adapter) buildThreadContext(ctx context.Context, channelID string, threadTS string) (string, error) {
	messages, err := a.client.ConversationsReplies(ctx, channelID, threadTS)
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(messages))
	for _, item := range messages {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		author := "assistant"
		if item.User != "" {
			author = "<@" + item.User + ">"
		} else if item.BotID == "" {
			author = "unknown"
		}
		lines = append(lines, author+": "+text)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return strings.Join(lines, "\n"), nil
}

func (a *Adapter) uploadArtifacts(ctx context.Context, root string, channelID string, threadTS string) error {
	items := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		items = append(items, path)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	sort.Strings(items)
	for _, path := range items {
		bytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(bytes) == 0 {
			continue
		}
		if err := a.client.UploadFile(ctx, UploadFileRequest{
			ChannelID: channelID,
			ThreadTS:  threadTS,
			Title:     filepath.Base(path),
			Filename:  filepath.Base(path),
			Bytes:     bytes,
		}); err != nil {
			return err
		}
	}
	return nil
}

type finalRenderer struct {
	client   Client
	channel  string
	threadTS string
	buffer   strings.Builder
}

func newFinalRenderer(client Client, channel string, threadTS string) *finalRenderer {
	return &finalRenderer{client: client, channel: channel, threadTS: threadTS}
}

func (r *finalRenderer) Emit(event engine.Event) error {
	if event.Kind == engine.EventTextDelta && strings.TrimSpace(event.TextDelta) != "" {
		r.buffer.WriteString(event.TextDelta)
	}
	return nil
}

func (r *finalRenderer) Finish(ctx context.Context, result ingress.HandleResult) error {
	texts := make([]string, 0, len(result.Messages))
	for _, outbound := range result.Messages {
		text := strings.TrimSpace(outbound.Text)
		if text != "" {
			texts = append(texts, text)
		}
	}
	if len(texts) == 0 {
		text := strings.TrimSpace(r.buffer.String())
		if text == "" {
			text = "I processed your request, but there was no text to display."
		}
		texts = append(texts, text)
	}
	for _, text := range texts {
		for _, rendered := range markdownToSlackMessages(text) {
			if err := r.client.PostMessage(ctx, PostMessageRequest{
				Channel:  r.channel,
				ThreadTS: r.threadTS,
				Text:     rendered.Text,
				Blocks:   rendered.Blocks,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func defaultSuggestedPrompts() []SuggestedPrompt {
	return []SuggestedPrompt{
		{Title: "Summarize thread", Message: "Summarize the current thread and highlight decisions."},
		{Title: "Review changes", Message: "Review the latest changes and call out risks."},
		{Title: "Plan next steps", Message: "Plan the next implementation steps for this discussion."},
	}
}

func decoratePrompt(question string, threadContext string, artifactDir string, binding ingress.SlackContextBinding) string {
	parts := []string{
		"Artifact handoff:",
		"- Only place files you want shared back to the Slack user in: " + artifactDir,
		"- Do not place throwaway, scratch, cache, or intermediate files there.",
		"- Keep artifacts for this run only.",
	}
	if strings.TrimSpace(threadContext) != "" {
		parts = append(parts,
			"Slack thread context:",
			threadContext,
		)
	}
	if len(binding.Metadata) > 0 {
		keys := make([]string, 0, len(binding.Metadata))
		for key := range binding.Metadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		lines := []string{"Assistant thread context:"}
		for _, key := range keys {
			lines = append(lines, key+": "+binding.Metadata[key])
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	parts = append(parts, "Request:", question)
	return strings.Join(parts, "\n\n")
}

func normalizeEventText(event Event) string {
	text := strings.TrimSpace(event.Text)
	if event.Type == "app_mention" {
		fields := strings.Fields(text)
		filtered := fields[:0]
		for _, field := range fields {
			if strings.HasPrefix(field, "<@") && strings.HasSuffix(field, ">") {
				continue
			}
			filtered = append(filtered, field)
		}
		text = strings.TrimSpace(strings.Join(filtered, " "))
	}
	return text
}

func threadRootTS(event Event) string {
	if strings.TrimSpace(event.ThreadTS) != "" {
		return strings.TrimSpace(event.ThreadTS)
	}
	return strings.TrimSpace(event.TS)
}

func chatTypeForEvent(event Event) ingress.ChatType {
	if event.ChannelType == "im" || strings.HasPrefix(strings.TrimSpace(event.Channel), "D") {
		return ingress.ChatTypeDM
	}
	return ingress.ChatTypeGroup
}

func buildRouteKey(teamID string, channelID string, threadTS string, chatType ingress.ChatType, assistant bool) string {
	threadTS = strings.TrimSpace(threadTS)
	if threadTS == "" {
		threadTS = channelID
	}
	switch {
	case assistant:
		return fmt.Sprintf("slack:assistant:%s:%s:%s", teamID, channelID, threadTS)
	case chatType == ingress.ChatTypeDM:
		return fmt.Sprintf("slack:im:%s:%s:%s", teamID, channelID, threadTS)
	default:
		return fmt.Sprintf("slack:thread:%s:%s:%s", teamID, channelID, threadTS)
	}
}

func summarizeTitle(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 80 {
		return text
	}
	return strings.TrimSpace(text[:77]) + "..."
}

func sanitizePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "run"
	}
	value = strings.ReplaceAll(value, "/", "_")
	value = strings.ReplaceAll(value, ":", "_")
	return value
}

func stringField(raw map[string]any, key string) string {
	if value, ok := raw[key].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

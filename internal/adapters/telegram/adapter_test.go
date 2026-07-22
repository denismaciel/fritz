package telegram

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fritz/internal/engine"
	"fritz/internal/ingress"
	"fritz/internal/tool"
	"fritz/internal/transcription"
)

func TestNormalizeUpdatePrivateMessage(t *testing.T) {
	msg, ok := NormalizeUpdate(Update{
		UpdateID: 10,
		Message: &Message{
			Chat: Chat{ID: 42, Type: "private"},
			From: &User{ID: 7},
			Text: "hi",
		},
	})
	if !ok {
		t.Fatal("NormalizeUpdate() ok = false")
	}
	if msg.Channel != "telegram" || msg.ChatType != ingress.ChatTypeDM || msg.UserID != "7" || msg.ChatID != "42" || msg.Text != "hi" {
		t.Fatalf("NormalizeUpdate() = %#v", msg)
	}
}

func TestNormalizeUpdateGroupMessageIncludesMetadata(t *testing.T) {
	msg, ok := NormalizeUpdate(Update{
		UpdateID: 11,
		Message: &Message{
			Chat: Chat{ID: 99, Type: "group", Title: "grp"},
			From: &User{ID: 5, Username: "alice", FirstName: "Alice", LastName: "Example"},
			Text: "photo",
			Document: &Document{
				FileName: "x.txt",
				MimeType: "text/plain",
			},
		},
	})
	if !ok {
		t.Fatal("NormalizeUpdate() ok = false")
	}
	if msg.ChatType != ingress.ChatTypeGroup {
		t.Fatalf("ChatType = %q", msg.ChatType)
	}
	if msg.Metadata["chat_title"] != "grp" || msg.Metadata["from_username"] != "alice" || msg.Metadata["from_name"] != "Alice Example" || msg.Metadata["document_name"] != "x.txt" {
		t.Fatalf("Metadata = %#v", msg.Metadata)
	}
}

func TestAdapterPollOnceTranscribesVoiceMessage(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat:  Chat{ID: 42, Type: "private"},
					From:  &User{ID: 7},
					Voice: &Voice{FileID: "voice-1", MimeType: "audio/ogg", Duration: 4},
				},
			},
		},
		filePathByID: map[string]string{
			"voice-1": "voice/path.ogg",
		},
		fileBodyByPath: map[string][]byte{
			"voice/path.ogg": []byte("ogg-bytes"),
		},
	}
	handler := &captureHandler{
		result: ingress.HandleResult{
			Messages: []ingress.OutboundMessage{{
				Channel: "telegram",
				ChatID:  "42",
				Text:    "pong",
			}},
		},
	}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
		Transcriber:  fakeTranscriber{text: "hello from voice"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if handler.calls != 1 {
		t.Fatalf("calls = %d", handler.calls)
	}
	if handler.last.Text != "[Audio]\nTranscript:\nhello from voice" {
		t.Fatalf("text = %q", handler.last.Text)
	}
	if handler.last.Metadata["audio_mime"] != "audio/ogg" || handler.last.Metadata["voice_duration"] != "4" {
		t.Fatalf("metadata = %#v", handler.last.Metadata)
	}
}

func TestAdapterPollOnceClearsSession(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{updates: []Update{{
		UpdateID: 3,
		Message:  &Message{Chat: Chat{ID: 42, Type: "private"}, From: &User{ID: 7}, Text: "/clear"},
	}}}
	handler := &clearingHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{AllowedUsers: []string{"7"}})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 || handler.sessionKey != "telegram:dm:7" {
		t.Fatalf("count = %d, sessionKey = %q", count, handler.sessionKey)
	}
	if len(client.sent) != 1 || client.sent[0].Text != "history cleared" {
		t.Fatalf("sent = %#v", client.sent)
	}
}

func TestAdapterPollOnceHandlesTrainingCommandsWithoutModel(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{updates: []Update{
		{
			UpdateID: 3,
			Message:  &Message{Chat: Chat{ID: 42, Type: "private"}, From: &User{ID: 7}, Text: "/training today"},
		},
		{
			UpdateID: 4,
			Message:  &Message{Chat: Chat{ID: 42, Type: "private"}, From: &User{ID: 7}, Text: "/training for the week"},
		},
	}}
	handler := &countingHandler{}
	training := &fakeTrainingProvider{today: "today plan", week: "week plan"}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		AllowedUsers: []string{"7"},
		Training:     training,
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 2 || handler.calls != 0 {
		t.Fatalf("count = %d, handler calls = %d", count, handler.calls)
	}
	if training.todayCalls != 1 || training.weekCalls != 1 {
		t.Fatalf("training calls = today %d, week %d", training.todayCalls, training.weekCalls)
	}
	if len(client.sent) != 2 || client.sent[0].Text != "today plan" || client.sent[1].Text != "week plan" {
		t.Fatalf("sent = %#v", client.sent)
	}
}

func TestAdapterTrainingCommandTargetsThisBotInGroup(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		me: BotInfo{Username: "denis_fritz_bot"},
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat: Chat{ID: 42, Type: "group"}, From: &User{ID: 7}, Text: "/training@other_bot week",
				},
			},
			{
				UpdateID: 4,
				Message: &Message{
					Chat: Chat{ID: 42, Type: "group"}, From: &User{ID: 7}, Text: "/training@denis_fritz_bot week",
				},
			},
		},
	}
	training := &fakeTrainingProvider{week: "week plan"}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, &countingHandler{}, Config{
		AllowedUsers: []string{"7"},
		Training:     training,
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 2 || training.weekCalls != 1 {
		t.Fatalf("count = %d, week calls = %d", count, training.weekCalls)
	}
	if len(client.sent) != 1 || client.sent[0].Text != "week plan" {
		t.Fatalf("sent = %#v", client.sent)
	}
}

func TestAdapterConfiguresTelegramCommandMenu(t *testing.T) {
	client := &fakeClient{}
	adapter := NewAdapter(filepath.Join(t.TempDir(), "telegram"), client, &countingHandler{}, Config{
		Training: &fakeTrainingProvider{},
	})

	if err := adapter.ConfigureCommands(context.Background()); err != nil {
		t.Fatalf("ConfigureCommands() error = %v", err)
	}
	if len(client.commands) != 3 {
		t.Fatalf("commands = %#v", client.commands)
	}
	if client.commands[1].Command != "training" || client.commands[2].Command != "training_week" {
		t.Fatalf("commands = %#v", client.commands)
	}
}

func TestAdapterPollOnceKeepsCaptionAndTranscriptForVoiceMessage(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat:    Chat{ID: 42, Type: "private"},
					From:    &User{ID: 7},
					Caption: "/remember this",
					Voice:   &Voice{FileID: "voice-1", MimeType: "audio/ogg"},
				},
			},
		},
		filePathByID: map[string]string{
			"voice-1": "voice/path.ogg",
		},
		fileBodyByPath: map[string][]byte{
			"voice/path.ogg": []byte("ogg-bytes"),
		},
	}
	handler := &captureHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
		Transcriber:  fakeTranscriber{text: "voice transcript"},
	})

	_, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	want := "[Audio]\nUser text:\n/remember this\nTranscript:\nvoice transcript"
	if handler.last.Text != want {
		t.Fatalf("text = %q", handler.last.Text)
	}
}

func TestAdapterPollOnceDownloadsPhotoMessage(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat:    Chat{ID: 42, Type: "private"},
					From:    &User{ID: 7},
					Caption: "what is this?",
					Photo: []Photo{
						{FileID: "small"},
						{FileID: "large"},
					},
				},
			},
		},
		filePathByID: map[string]string{
			"large": "photos/file.png",
		},
		fileBodyByPath: map[string][]byte{
			"photos/file.png": []byte("png-bytes"),
		},
	}
	handler := &captureHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if handler.calls != 1 {
		t.Fatalf("calls = %d", handler.calls)
	}
	if handler.last.Text != "what is this?" {
		t.Fatalf("text = %q", handler.last.Text)
	}
	if handler.last.Metadata["photo_count"] != "2" || handler.last.Metadata["image_count"] != "1" {
		t.Fatalf("metadata = %#v", handler.last.Metadata)
	}
	if len(handler.last.Images) != 1 {
		t.Fatalf("images = %#v", handler.last.Images)
	}
	image := handler.last.Images[0]
	if image.Kind != tool.ImagePartKind || image.MIMEType != "image/png" || image.Data != base64.StdEncoding.EncodeToString([]byte("png-bytes")) {
		t.Fatalf("image = %#v", image)
	}
}

func TestAdapterPollOnceHandlesPhotoOnlyMessage(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat:  Chat{ID: 42, Type: "private"},
					From:  &User{ID: 7},
					Photo: []Photo{{FileID: "photo-1"}},
				},
			},
		},
		filePathByID: map[string]string{
			"photo-1": "photos/file.jpg",
		},
		fileBodyByPath: map[string][]byte{
			"photos/file.jpg": []byte("jpg-bytes"),
		},
	}
	handler := &captureHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if handler.last.Text != "[Image]" {
		t.Fatalf("text = %q", handler.last.Text)
	}
	if len(handler.last.Images) != 1 || handler.last.Images[0].MIMEType != "image/jpeg" {
		t.Fatalf("images = %#v", handler.last.Images)
	}
}

func TestAdapterPollOnceSkipsVoiceOnlyMessageWhenTranscriptionFails(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat:  Chat{ID: 42, Type: "private"},
					From:  &User{ID: 7},
					Voice: &Voice{FileID: "voice-1", MimeType: "audio/ogg"},
				},
			},
		},
		filePathByID: map[string]string{
			"voice-1": "voice/path.ogg",
		},
		fileBodyByPath: map[string][]byte{
			"voice/path.ogg": []byte("ogg-bytes"),
		},
	}
	handler := &captureHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
		Transcriber:  fakeTranscriber{err: errors.New("boom")},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d", count)
	}
	if handler.calls != 0 {
		t.Fatalf("calls = %d", handler.calls)
	}
}

func TestAdapterPollOnceRoutesRepliesAndPersistsOffset(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 3,
				Message: &Message{
					Chat: Chat{ID: 42, Type: "private"},
					From: &User{ID: 7},
					Text: "hi",
				},
			},
		},
	}
	handler := fakeHandler{
		result: ingress.HandleResult{
			SessionKey: "telegram:dm:7",
			Session:    engine.SessionRef{ID: "s1", Path: "/tmp/s1.jsonl"},
			Messages: []ingress.OutboundMessage{{
				Channel: "telegram",
				ChatID:  "42",
				Text:    "pong",
			}},
		},
	}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if len(client.sent) != 1 || client.sent[0].ChatID != 42 || client.sent[0].Text != "pong" {
		t.Fatalf("sent = %#v", client.sent)
	}
	if client.lastOffset != 0 {
		t.Fatalf("lastOffset = %d", client.lastOffset)
	}
	if len(client.lastAllowed) != 1 || client.lastAllowed[0] != "message" {
		t.Fatalf("allowed updates = %#v", client.lastAllowed)
	}

	client.updates = nil
	secondAdapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})
	_, err = secondAdapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("second PollOnce() error = %v", err)
	}
	if client.lastOffset != 4 {
		t.Fatalf("persisted offset = %d", client.lastOffset)
	}
	if _, err := os.Stat(filepath.Join(dir, "telegram", "offset.json")); err != nil {
		t.Fatalf("offset stat error = %v", err)
	}
}

func TestAdapterBlocksUnauthorizedDM(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{{
			UpdateID: 1,
			Message: &Message{
				Chat: Chat{ID: 42, Type: "private"},
				From: &User{ID: 7},
				Text: "hi",
			},
		}},
	}
	handler := &countingHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{PollTimeout: time.Second})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if handler.calls != 0 {
		t.Fatalf("calls = %d", handler.calls)
	}
	if len(client.sent) != 1 || client.sent[0].Text != "not authorized" {
		t.Fatalf("sent = %#v", client.sent)
	}
}

func TestAdapterPairsThenAllowsUser(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{{
			UpdateID: 1,
			Message: &Message{
				Chat: Chat{ID: 42, Type: "private"},
				From: &User{ID: 7},
				Text: "/start secret",
			},
		}},
	}
	handler := &countingHandler{
		result: ingress.HandleResult{
			Messages: []ingress.OutboundMessage{{
				Channel: "telegram",
				ChatID:  "42",
				Text:    "pong",
			}},
		},
	}

	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		PairingToken: "secret",
		AllowedUsers: nil,
	})
	_, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("pair PollOnce() error = %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("calls after pair = %d", handler.calls)
	}
	if len(client.sent) != 1 || client.sent[0].Text != "paired" {
		t.Fatalf("sent after pair = %#v", client.sent)
	}
	if _, err := os.Stat(filepath.Join(dir, "telegram", "allowlist.json")); err != nil {
		t.Fatalf("allowlist stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "telegram", "pairing.json")); err != nil {
		t.Fatalf("pairing stat error = %v", err)
	}

	client.updates = []Update{{
		UpdateID: 2,
		Message: &Message{
			Chat: Chat{ID: 42, Type: "private"},
			From: &User{ID: 7},
			Text: "hi",
		},
	}}
	client.sent = nil
	secondAdapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		PairingToken: "secret",
	})
	_, err = secondAdapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("authorized PollOnce() error = %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("calls after auth = %d", handler.calls)
	}
	if len(client.sent) != 1 || client.sent[0].Text != "pong" {
		t.Fatalf("sent after auth = %#v", client.sent)
	}
}

func TestAdapterCachesUnauthorizedGroupContextWithoutTriggering(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		me: BotInfo{Username: "borinho_bot"},
		updates: []Update{{
			UpdateID: 1,
			Message: &Message{
				MessageID: 10,
				Chat:      Chat{ID: 99, Type: "group"},
				From:      &User{ID: 7, FirstName: "Chode"},
				Date:      100,
				Text:      "side comment",
			},
		}},
	}
	handler := &countingHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{PollTimeout: time.Second})

	_, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if handler.calls != 0 {
		t.Fatalf("calls = %d", handler.calls)
	}
	if len(client.sent) != 0 {
		t.Fatalf("sent = %#v", client.sent)
	}
	state, err := adapter.loadGroupContext("99")
	if err != nil {
		t.Fatalf("loadGroupContext() error = %v", err)
	}
	if len(state.Messages) != 1 || state.Messages[0].Text != "side comment" || state.Messages[0].Name != "Chode" {
		t.Fatalf("messages = %#v", state.Messages)
	}

	client.updates = []Update{{
		UpdateID: 2,
		Message: &Message{
			MessageID: 11,
			Chat:      Chat{ID: 99, Type: "group"},
			From:      &User{ID: 8, Username: "denis"},
			Date:      101,
			Text:      "@borinho_bot what did chode say?",
		},
	}}
	handlerWithCapture := &captureHandler{}
	secondAdapter := NewAdapter(filepath.Join(dir, "telegram"), client, handlerWithCapture, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"8"},
	})
	if _, err := secondAdapter.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce() error = %v", err)
	}
	if handlerWithCapture.calls != 1 {
		t.Fatalf("calls = %d", handlerWithCapture.calls)
	}
	if !strings.Contains(handlerWithCapture.last.Text, "user Chode (id 7): side comment") {
		t.Fatalf("decorated prompt = %q", handlerWithCapture.last.Text)
	}
}

func TestAdapterBuffersGroupContextAndOnlyRepliesWhenAddressed(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{
			{
				UpdateID: 1,
				Message: &Message{
					MessageID: 10,
					Chat:      Chat{ID: 99, Type: "group", Title: "grp"},
					From:      &User{ID: 7, Username: "alice"},
					Date:      100,
					Text:      "we need pizza",
				},
			},
			{
				UpdateID: 2,
				Message: &Message{
					MessageID: 11,
					Chat:      Chat{ID: 99, Type: "group", Title: "grp"},
					From:      &User{ID: 8, Username: "bob"},
					Date:      101,
					Text:      "@borinho_bot summarize",
				},
			},
		},
		me: BotInfo{Username: "borinho_bot"},
	}
	handler := &captureHandler{
		result: ingress.HandleResult{
			SessionKey: "telegram:group:99",
			Messages: []ingress.OutboundMessage{{
				Channel: "telegram",
				ChatID:  "99",
				Text:    "pong",
			}},
		},
	}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7", "8"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}
	if handler.calls != 1 {
		t.Fatalf("calls = %d", handler.calls)
	}
	if !strings.Contains(handler.last.Text, "Telegram group context") ||
		!strings.Contains(handler.last.Text, "user alice (id 7): we need pizza") ||
		!strings.Contains(handler.last.Text, "Addressed request:") ||
		!strings.Contains(handler.last.Text, "user bob (id 8): summarize") ||
		strings.Contains(handler.last.Text, "@borinho_bot") {
		t.Fatalf("decorated prompt = %q", handler.last.Text)
	}
	if len(client.sent) != 1 || client.sent[0].ChatID != 99 || client.sent[0].Text != "pong" {
		t.Fatalf("sent = %#v", client.sent)
	}
	state, err := adapter.loadGroupContext("99")
	if err != nil {
		t.Fatalf("loadGroupContext() error = %v", err)
	}
	if len(state.Messages) != 3 {
		t.Fatalf("messages = %#v", state.Messages)
	}
	if state.Messages[1].Text != "@borinho_bot summarize" || strings.Contains(state.Messages[1].Text, "Telegram group context") {
		t.Fatalf("stored addressed message = %#v", state.Messages[1])
	}
	if state.Messages[2].UserID != "fritz" || state.Messages[2].Text != "pong" {
		t.Fatalf("stored reply = %#v", state.Messages[2])
	}
}

func TestSendReplySplitsLongMessages(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{}
	handler := fakeHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{})

	err := adapter.sendReply(context.Background(), "42", strings.Repeat("hello ", 900))
	if err != nil {
		t.Fatalf("sendReply() error = %v", err)
	}
	if len(client.sent) < 2 {
		t.Fatalf("sent = %#v", client.sent)
	}
	for _, req := range client.sent {
		if req.ChatID != 42 {
			t.Fatalf("chat id = %d", req.ChatID)
		}
		if len([]rune(req.Text)) > telegramMaxMessageRunes {
			t.Fatalf("message too long: %d", len([]rune(req.Text)))
		}
	}
}

func TestSendReplyFallsBackToPlainTextWhenHTMLExpansionIsTooLarge(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{}
	handler := fakeHandler{}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{})

	err := adapter.sendReply(context.Background(), "42", strings.Repeat("&", telegramReplyChunkRunes))
	if err != nil {
		t.Fatalf("sendReply() error = %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent = %#v", client.sent)
	}
	if client.sent[0].ParseMode != "" {
		t.Fatalf("parse mode = %q", client.sent[0].ParseMode)
	}
	if len([]rune(client.sent[0].Text)) > telegramMaxMessageRunes {
		t.Fatalf("message too long: %d", len([]rune(client.sent[0].Text)))
	}
}

func TestPollOnceSendsErrorReplyWhenHandlerFails(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{{
			UpdateID: 3,
			Message: &Message{
				Chat: Chat{ID: 42, Type: "private"},
				From: &User{ID: 7},
				Text: "hi",
			},
		}},
	}
	handler := fakeHandler{err: errors.New("codex request failed")}
	adapter := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})

	count, err := adapter.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
	if len(client.sent) != 1 || !strings.Contains(client.sent[0].Text, "codex request failed") {
		t.Fatalf("sent = %#v", client.sent)
	}

	client.updates = nil
	again := NewAdapter(filepath.Join(dir, "telegram"), client, handler, Config{
		PollTimeout:  time.Second,
		AllowedUsers: []string{"7"},
	})
	if _, err := again.PollOnce(context.Background()); err != nil {
		t.Fatalf("second PollOnce() error = %v", err)
	}
	if client.lastOffset != 4 {
		t.Fatalf("lastOffset = %d", client.lastOffset)
	}
}

type fakeClient struct {
	updates        []Update
	lastOffset     int64
	lastTimeout    int
	lastAllowed    []string
	sent           []SendMessageRequest
	filePathByID   map[string]string
	fileBodyByPath map[string][]byte
	me             BotInfo
	commands       []BotCommand
	deletedWebhook bool
}

func (f *fakeClient) DeleteWebhook(context.Context, DeleteWebhookRequest) error {
	f.deletedWebhook = true
	return nil
}

func (f *fakeClient) GetMe(context.Context) (BotInfo, error) {
	if f.me.ID == 0 && f.me.Username == "" {
		return BotInfo{Username: "fritz"}, nil
	}
	return f.me, nil
}

func (f *fakeClient) GetUpdates(_ context.Context, req GetUpdatesRequest) ([]Update, error) {
	f.lastOffset = req.Offset
	f.lastTimeout = req.TimeoutSeconds
	f.lastAllowed = append([]string(nil), req.AllowedUpdates...)
	return append([]Update(nil), f.updates...), nil
}

func (f *fakeClient) SendMessage(_ context.Context, req SendMessageRequest) error {
	f.sent = append(f.sent, req)
	return nil
}

func (f *fakeClient) SetMyCommands(_ context.Context, req SetMyCommandsRequest) error {
	f.commands = append([]BotCommand(nil), req.Commands...)
	return nil
}

func (f *fakeClient) GetFile(_ context.Context, fileID string) (File, error) {
	path := f.filePathByID[fileID]
	if path == "" {
		return File{}, errors.New("missing file")
	}
	return File{FileID: fileID, FilePath: path}, nil
}

func (f *fakeClient) DownloadFile(_ context.Context, filePath string) ([]byte, error) {
	body, ok := f.fileBodyByPath[filePath]
	if !ok {
		return nil, errors.New("missing file body")
	}
	return append([]byte(nil), body...), nil
}

type fakeHandler struct {
	result ingress.HandleResult
	err    error
}

type fakeTrainingProvider struct {
	today      string
	week       string
	err        error
	todayCalls int
	weekCalls  int
}

func (f *fakeTrainingProvider) Today(context.Context, time.Time) (string, error) {
	f.todayCalls++
	return f.today, f.err
}

func (f *fakeTrainingProvider) Week(context.Context, time.Time) (string, error) {
	f.weekCalls++
	return f.week, f.err
}

type clearingHandler struct {
	sessionKey string
}

func (h *clearingHandler) HandleInbound(context.Context, ingress.InboundMessage) (ingress.HandleResult, error) {
	return ingress.HandleResult{}, nil
}

func (h *clearingHandler) ClearSessionKey(_ context.Context, sessionKey string) error {
	h.sessionKey = sessionKey
	return nil
}

func (f fakeHandler) HandleInbound(_ context.Context, _ ingress.InboundMessage) (ingress.HandleResult, error) {
	return f.result, f.err
}

type countingHandler struct {
	result ingress.HandleResult
	err    error
	calls  int
}

func (f *countingHandler) HandleInbound(_ context.Context, _ ingress.InboundMessage) (ingress.HandleResult, error) {
	f.calls++
	return f.result, f.err
}

type captureHandler struct {
	result ingress.HandleResult
	err    error
	calls  int
	last   ingress.InboundMessage
}

func (f *captureHandler) HandleInbound(_ context.Context, msg ingress.InboundMessage) (ingress.HandleResult, error) {
	f.calls++
	f.last = msg
	return f.result, f.err
}

type fakeTranscriber struct {
	text string
	err  error
}

func (f fakeTranscriber) Transcribe(context.Context, transcription.AudioInput) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.text, nil
}

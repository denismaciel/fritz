package telegram

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
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
			From: &User{ID: 5, Username: "alice"},
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
	if msg.Metadata["chat_title"] != "grp" || msg.Metadata["from_username"] != "alice" || msg.Metadata["document_name"] != "x.txt" {
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

func TestAdapterIgnoresUnauthorizedGroup(t *testing.T) {
	dir := t.TempDir()
	client := &fakeClient{
		updates: []Update{{
			UpdateID: 1,
			Message: &Message{
				Chat: Chat{ID: 99, Type: "group"},
				From: &User{ID: 7},
				Text: "hi",
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
}

type fakeClient struct {
	updates        []Update
	lastOffset     int64
	lastTimeout    int
	sent           []SendMessageRequest
	filePathByID   map[string]string
	fileBodyByPath map[string][]byte
}

func (f *fakeClient) GetUpdates(_ context.Context, req GetUpdatesRequest) ([]Update, error) {
	f.lastOffset = req.Offset
	f.lastTimeout = req.TimeoutSeconds
	return append([]Update(nil), f.updates...), nil
}

func (f *fakeClient) SendMessage(_ context.Context, req SendMessageRequest) error {
	f.sent = append(f.sent, req)
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

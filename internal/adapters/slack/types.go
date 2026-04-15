package slack

import "encoding/json"

type SocketEnvelope struct {
	EnvelopeID             string          `json:"envelope_id"`
	Type                   string          `json:"type"`
	AcceptsResponsePayload bool            `json:"accepts_response_payload"`
	Payload                json.RawMessage `json:"payload"`
}

type EventsAPIEnvelope struct {
	Token     string          `json:"token"`
	TeamID    string          `json:"team_id"`
	Type      string          `json:"type"`
	EventID   string          `json:"event_id"`
	EventTime int64           `json:"event_time"`
	Event     json.RawMessage `json:"event"`
}

type Event struct {
	Type        string               `json:"type"`
	Subtype     string               `json:"subtype,omitempty"`
	User        string               `json:"user,omitempty"`
	Text        string               `json:"text,omitempty"`
	Channel     string               `json:"channel,omitempty"`
	ChannelType string               `json:"channel_type,omitempty"`
	ThreadTS    string               `json:"thread_ts,omitempty"`
	TS          string               `json:"ts,omitempty"`
	EventTS     string               `json:"event_ts,omitempty"`
	BotID       string               `json:"bot_id,omitempty"`
	Context     map[string]any       `json:"context,omitempty"`
	Assistant   *AssistantThreadInfo `json:"assistant_thread,omitempty"`
}

type AssistantThreadInfo struct {
	ChannelID string         `json:"channel_id,omitempty"`
	ThreadTS  string         `json:"thread_ts,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
}

type HistoryMessage struct {
	User     string `json:"user,omitempty"`
	Text     string `json:"text,omitempty"`
	TS       string `json:"ts,omitempty"`
	ThreadTS string `json:"thread_ts,omitempty"`
	Subtype  string `json:"subtype,omitempty"`
	BotID    string `json:"bot_id,omitempty"`
}

type PostMessageRequest struct {
	Channel  string           `json:"channel"`
	Text     string           `json:"text"`
	ThreadTS string           `json:"thread_ts,omitempty"`
	Blocks   []map[string]any `json:"blocks,omitempty"`
}

type StreamHandle struct {
	Channel  string
	ThreadTS string
	TS       string
}

type AssistantThreadRef struct {
	ChannelID string
	ThreadTS  string
}

type SuggestedPrompt struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}

type UploadFileRequest struct {
	ChannelID string
	ThreadTS  string
	Title     string
	Filename  string
	Bytes     []byte
}

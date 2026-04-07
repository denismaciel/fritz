package telegram

import (
	"context"
	"testing"

	"fritz/internal/heartbeat"
)

func TestSenderSendsMessage(t *testing.T) {
	client := &fakeClient{}
	sender := NewSender(client)
	err := sender.Send(context.Background(), heartbeat.Wake{ChatID: "42"}, "hi")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(client.sent) != 1 || client.sent[0].ChatID != 42 || client.sent[0].Text != "hi" {
		t.Fatalf("sent = %#v", client.sent)
	}
}

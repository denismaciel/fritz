package telegram

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactTelegramError(t *testing.T) {
	err := redactTelegramError(errors.New(`Post "https://api.telegram.org/bot123456:secret/getUpdates": context canceled`))
	text := err.Error()
	if strings.Contains(text, "123456:secret") {
		t.Fatalf("token leaked: %s", text)
	}
	if !strings.Contains(text, "/botREDACTED/getUpdates") {
		t.Fatalf("redacted error = %s", text)
	}
}

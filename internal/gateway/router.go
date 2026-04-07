package gateway

import (
	"fmt"
	"strings"
)

type Router interface {
	SessionKey(InboundMessage) (string, error)
}

type DefaultRouter struct{}

func (DefaultRouter) SessionKey(message InboundMessage) (string, error) {
	if key := strings.TrimSpace(message.SessionKey); key != "" {
		return key, nil
	}
	channel := strings.TrimSpace(message.Channel)
	if channel == "" {
		return "", fmt.Errorf("missing channel")
	}
	switch message.ChatType {
	case ChatTypeDM:
		if userID := strings.TrimSpace(message.UserID); userID != "" {
			return channel + ":dm:" + userID, nil
		}
		if chatID := strings.TrimSpace(message.ChatID); chatID != "" {
			return channel + ":dm:" + chatID, nil
		}
		return "", fmt.Errorf("missing dm user id")
	case ChatTypeGroup:
		if chatID := strings.TrimSpace(message.ChatID); chatID != "" {
			return channel + ":group:" + chatID, nil
		}
		return "", fmt.Errorf("missing group chat id")
	default:
		if chatID := strings.TrimSpace(message.ChatID); chatID != "" {
			return channel + ":" + string(message.ChatType) + ":" + chatID, nil
		}
		if userID := strings.TrimSpace(message.UserID); userID != "" {
			return channel + ":" + string(message.ChatType) + ":" + userID, nil
		}
		return "", fmt.Errorf("missing route id")
	}
}

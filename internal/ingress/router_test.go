package ingress

import "testing"

func TestDefaultRouterUsesExplicitSessionKey(t *testing.T) {
	key, err := DefaultRouter{}.SessionKey(InboundMessage{
		Channel:    "telegram",
		ChatType:   ChatTypeDM,
		UserID:     "u1",
		SessionKey: "custom:key",
	})
	if err != nil {
		t.Fatalf("SessionKey() error = %v", err)
	}
	if key != "custom:key" {
		t.Fatalf("SessionKey() = %q", key)
	}
}

func TestDefaultRouterMapsDMToUserSession(t *testing.T) {
	key, err := DefaultRouter{}.SessionKey(InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
		UserID:   "u1",
		ChatID:   "chat-1",
	})
	if err != nil {
		t.Fatalf("SessionKey() error = %v", err)
	}
	if key != "telegram:dm:u1" {
		t.Fatalf("SessionKey() = %q", key)
	}
}

func TestDefaultRouterMapsGroupToChatSession(t *testing.T) {
	key, err := DefaultRouter{}.SessionKey(InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeGroup,
		ChatID:   "g1",
		UserID:   "u1",
	})
	if err != nil {
		t.Fatalf("SessionKey() error = %v", err)
	}
	if key != "telegram:group:g1" {
		t.Fatalf("SessionKey() = %q", key)
	}
}

func TestDefaultRouterRejectsMissingDMIdentity(t *testing.T) {
	_, err := DefaultRouter{}.SessionKey(InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeDM,
	})
	if err == nil {
		t.Fatal("SessionKey() err = nil")
	}
}

func TestDefaultRouterRejectsMissingGroupIdentity(t *testing.T) {
	_, err := DefaultRouter{}.SessionKey(InboundMessage{
		Channel:  "telegram",
		ChatType: ChatTypeGroup,
		UserID:   "u1",
	})
	if err == nil {
		t.Fatal("SessionKey() err = nil")
	}
}

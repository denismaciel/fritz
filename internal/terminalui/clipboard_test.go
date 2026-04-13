package terminalui

import "testing"

func TestPreferredClipboardImageType(t *testing.T) {
	got := preferredClipboardImageType([]string{"text/plain", "image/webp", "image/png"})
	if got != "image/png" {
		t.Fatalf("got %q", got)
	}
}

func TestClipboardHasText(t *testing.T) {
	if !clipboardHasText([]string{"application/json", "text/plain;charset=utf-8"}) {
		t.Fatal("expected text clipboard type")
	}
	if clipboardHasText([]string{"image/png"}) {
		t.Fatal("unexpected text clipboard type")
	}
}

func TestIsEmptyClipboardError(t *testing.T) {
	if !isEmptyClipboardError(assertErr("wl-paste failed: Nothing is copied")) {
		t.Fatal("expected empty clipboard error")
	}
}

func assertErr(text string) error { return testErr(text) }

type testErr string

func (e testErr) Error() string { return string(e) }

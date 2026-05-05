package terminalui

import (
	"testing"
)

func TestShimmer(t *testing.T) {
	s := Shimmer("Loading")
	if s == "" {
		t.Fatal("expected non-empty string")
	}
}

func TestActivityIndicator(t *testing.T) {
	s := ActivityIndicator(shimmerStart)
	if s == "" {
		t.Fatal("expected non-empty string")
	}
}

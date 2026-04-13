package terminalui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"fritz/internal/tool"
)

type clipboardPasteResult struct {
	text  string
	image tool.ContentPart
}

var supportedClipboardImageTypes = []string{
	"image/png",
	"image/jpeg",
	"image/webp",
	"image/gif",
}

func readWaylandClipboard() (clipboardPasteResult, error) {
	types, err := waylandClipboardTypes()
	if err != nil {
		if isEmptyClipboardError(err) {
			return clipboardPasteResult{}, nil
		}
		return clipboardPasteResult{}, err
	}
	if mimeType := preferredClipboardImageType(types); mimeType != "" {
		data, err := waylandClipboardImageBytes(mimeType)
		if err != nil {
			return clipboardPasteResult{}, err
		}
		if len(data) == 0 {
			return clipboardPasteResult{}, nil
		}
		return clipboardPasteResult{
			image: tool.ImagePart(mimeType, base64.StdEncoding.EncodeToString(data)),
		}, nil
	}
	if !clipboardHasText(types) {
		return clipboardPasteResult{}, nil
	}
	data, err := runClipboardCommand("wl-paste", "--no-newline")
	if err != nil {
		if isEmptyClipboardError(err) {
			return clipboardPasteResult{}, nil
		}
		return clipboardPasteResult{}, err
	}
	return clipboardPasteResult{text: string(data)}, nil
}

func waylandClipboardTypes() ([]string, error) {
	out, err := runClipboardCommand("wl-paste", "--list-types")
	if err != nil {
		return nil, err
	}
	types := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i := range types {
		types[i] = strings.TrimSpace(types[i])
	}
	return types, nil
}

func preferredClipboardImageType(types []string) string {
	for _, supported := range supportedClipboardImageTypes {
		for _, candidate := range types {
			if candidate == supported {
				return supported
			}
		}
	}
	return ""
}

func clipboardHasText(types []string) bool {
	for _, candidate := range types {
		switch candidate {
		case "text/plain", "text/plain;charset=utf-8", "UTF8_STRING", "TEXT", "STRING":
			return true
		}
	}
	return false
}

func waylandClipboardImageBytes(mimeType string) ([]byte, error) {
	return runClipboardCommand("wl-paste", "--type", mimeType, "--no-newline")
}

func runClipboardCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("%s timed out", name)
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s failed: %s", name, msg)
	}
	return out, nil
}

func isEmptyClipboardError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Nothing is copied")
}

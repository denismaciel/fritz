package tool

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DefaultMaxLines   = 2000
	DefaultMaxBytes   = 50 * 1024
	GrepMaxLineLength = 500
	DefaultReadMaxBytes = 128 * 1024
	DefaultGrepLimit = 100
)

type TruncationResult struct {
	Content               string `json:"content"`
	Truncated             bool   `json:"truncated"`
	TruncatedBy           string `json:"truncated_by,omitempty"`
	TotalLines            int    `json:"total_lines"`
	TotalBytes            int    `json:"total_bytes"`
	OutputLines           int    `json:"output_lines"`
	OutputBytes           int    `json:"output_bytes"`
	LastLinePartial       bool   `json:"last_line_partial,omitempty"`
	FirstLineExceedsLimit bool   `json:"first_line_exceeds_limit,omitempty"`
	MaxLines              int    `json:"max_lines"`
	MaxBytes              int    `json:"max_bytes"`
}

type TruncationOptions struct {
	MaxLines int
	MaxBytes int
}

func FormatSize(bytes int) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

func TruncateHead(content string, options TruncationOptions) TruncationResult {
	maxLines := options.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := len([]byte(content))
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	if len(lines) > 0 && len([]byte(lines[0])) > maxBytes {
		return TruncationResult{
			Truncated:             true,
			TruncatedBy:           "bytes",
			TotalLines:            totalLines,
			TotalBytes:            totalBytes,
			MaxLines:              maxLines,
			MaxBytes:              maxBytes,
			FirstLineExceedsLimit: true,
		}
	}

	out := make([]string, 0, min(totalLines, maxLines))
	outBytes := 0
	truncatedBy := "lines"
	for i, line := range lines {
		if i >= maxLines {
			truncatedBy = "lines"
			break
		}
		lineBytes := len([]byte(line))
		if i > 0 {
			lineBytes++
		}
		if outBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			break
		}
		out = append(out, line)
		outBytes += lineBytes
	}

	outContent := strings.Join(out, "\n")
	return TruncationResult{
		Content:     outContent,
		Truncated:   true,
		TruncatedBy: truncatedBy,
		TotalLines:  totalLines,
		TotalBytes:  totalBytes,
		OutputLines: len(out),
		OutputBytes: len([]byte(outContent)),
		MaxLines:    maxLines,
		MaxBytes:    maxBytes,
	}
}

func TruncateTail(content string, options TruncationOptions) TruncationResult {
	maxLines := options.MaxLines
	if maxLines == 0 {
		maxLines = DefaultMaxLines
	}
	maxBytes := options.MaxBytes
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}

	totalBytes := len([]byte(content))
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	if totalLines <= maxLines && totalBytes <= maxBytes {
		return TruncationResult{
			Content:     content,
			TotalLines:  totalLines,
			TotalBytes:  totalBytes,
			OutputLines: totalLines,
			OutputBytes: totalBytes,
			MaxLines:    maxLines,
			MaxBytes:    maxBytes,
		}
	}

	out := make([]string, 0, min(totalLines, maxLines))
	outBytes := 0
	truncatedBy := "lines"
	lastLinePartial := false
	for i := len(lines) - 1; i >= 0; i-- {
		if len(out) >= maxLines {
			truncatedBy = "lines"
			break
		}
		line := lines[i]
		lineBytes := len([]byte(line))
		if len(out) > 0 {
			lineBytes++
		}
		if outBytes+lineBytes > maxBytes {
			truncatedBy = "bytes"
			if len(out) == 0 {
				out = append([]string{truncateStringToBytesFromEnd(line, maxBytes)}, out...)
				lastLinePartial = true
			}
			break
		}
		out = append([]string{line}, out...)
		outBytes += lineBytes
	}

	outContent := strings.Join(out, "\n")
	return TruncationResult{
		Content:         outContent,
		Truncated:       true,
		TruncatedBy:     truncatedBy,
		TotalLines:      totalLines,
		TotalBytes:      totalBytes,
		OutputLines:     len(out),
		OutputBytes:     len([]byte(outContent)),
		MaxLines:        maxLines,
		MaxBytes:        maxBytes,
		LastLinePartial: lastLinePartial,
	}
}

func TruncateLine(line string, maxChars int) (string, bool) {
	if maxChars == 0 {
		maxChars = GrepMaxLineLength
	}
	if utf8.RuneCountInString(line) <= maxChars {
		return line, false
	}
	runes := []rune(line)
	return string(runes[:maxChars]) + " [truncated]", true
}

func truncateStringToBytesFromEnd(text string, maxBytes int) string {
	data := []byte(text)
	if len(data) <= maxBytes {
		return text
	}
	start := len(data) - maxBytes
	for start < len(data) && (data[start]&0xc0) == 0x80 {
		start++
	}
	return string(data[start:])
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

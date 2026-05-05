package terminalui

import (
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

var shimmerStart = time.Now()
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func Shimmer(text string) string {
	if text == "" {
		return ""
	}

	chars := []rune(text)
	padding := 10
	period := len(chars) + padding*2
	sweepSeconds := 2.0
	elapsed := time.Since(shimmerStart).Seconds()
	posF := math.Mod(elapsed, sweepSeconds) / sweepSeconds * float64(period)
	pos := int(posF)

	bandHalfWidth := 5.0

	// Default foreground and highlight colors.
	// In Rust they used (128,128,128) and (255,255,255).
	baseColor, _ := colorful.Hex("#808080")
	highlightColor, _ := colorful.Hex("#ffffff")

	var builder strings.Builder
	for i, ch := range chars {
		iPos := float64(i + padding)
		dist := math.Abs(iPos - float64(pos))

		t := 0.0
		if dist <= bandHalfWidth {
			x := math.Pi * (dist / bandHalfWidth)
			t = 0.5 * (1.0 + math.Cos(x))
		}

		// Blend colors
		// highlight * 0.9 in Rust
		c := baseColor.BlendRgb(highlightColor, t*0.9)

		style := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Bold(true)
		builder.WriteString(style.Render(string(ch)))
	}

	return builder.String()
}

func ActivityIndicator(start time.Time) string {
	elapsed := time.Since(shimmerStart).Milliseconds()
	frameIdx := (elapsed / 80) % int64(len(spinnerFrames))
	return Shimmer(spinnerFrames[frameIdx])
}

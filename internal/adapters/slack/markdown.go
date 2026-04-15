package slack

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

const (
	maxSectionTextLength  = 3000
	maxBlocksPerMessage   = 50
	maxFallbackTextLength = 4000
)

type renderedSlackMessage struct {
	Text   string
	Blocks []map[string]any
}

var slackMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func markdownToSlackMessages(markdown string) []renderedSlackMessage {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return sectionsToMessages([]string{"I processed your request, but there was no text to display."})
	}

	reader := text.NewReader([]byte(trimmed))
	doc := slackMarkdown.Parser().Parse(reader)
	renderedSections := make([]string, 0)
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		for _, block := range renderBlockContent(node, []byte(trimmed), 0) {
			for _, chunk := range splitToMaxLength(block, maxSectionTextLength) {
				chunk = strings.TrimSpace(chunk)
				if chunk != "" {
					renderedSections = append(renderedSections, chunk)
				}
			}
		}
	}
	if len(renderedSections) == 0 {
		return sectionsToMessages(splitToMaxLength(trimmed, maxSectionTextLength))
	}
	return sectionsToMessages(renderedSections)
}

func escapeSlackText(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	return strings.ReplaceAll(value, ">", "&gt;")
}

func wrapInline(marker string, content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	return marker + trimmed + marker
}

func renderInlineChildren(parent gast.Node, source []byte) string {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(renderInlineContent(child, source))
	}
	return out.String()
}

func renderInlineContent(node gast.Node, source []byte) string {
	switch typed := node.(type) {
	case *gast.Text:
		var out strings.Builder
		out.WriteString(escapeSlackText(string(typed.Segment.Value(source))))
		if typed.HardLineBreak() || typed.SoftLineBreak() {
			out.WriteString("\n")
		}
		return out.String()
	case *gast.String:
		return escapeSlackText(string(typed.Value))
	case *gast.CodeSpan:
		return "`" + escapeSlackText(renderInlineChildren(typed, source)) + "`"
	case *gast.Emphasis:
		marker := "_"
		if typed.Level >= 2 {
			marker = "*"
		}
		return wrapInline(marker, renderInlineChildren(typed, source))
	case *extast.Strikethrough:
		return wrapInline("~", renderInlineChildren(typed, source))
	case *gast.Link:
		label := strings.TrimSpace(renderInlineChildren(typed, source))
		url := strings.TrimSpace(string(typed.Destination))
		if label == "" {
			return "<" + url + ">"
		}
		return "<" + url + "|" + label + ">"
	case *gast.AutoLink:
		label := strings.TrimSpace(string(typed.Label(source)))
		url := strings.TrimSpace(string(typed.URL(source)))
		if label == "" || label == url {
			return "<" + url + ">"
		}
		return "<" + url + "|" + escapeSlackText(label) + ">"
	case *gast.Image:
		alt := strings.TrimSpace(renderInlineChildren(typed, source))
		url := strings.TrimSpace(string(typed.Destination))
		if alt == "" {
			return "<" + url + ">"
		}
		return "<" + url + "|" + alt + ">"
	default:
		return renderInlineChildren(node, source)
	}
}

func renderParagraph(node *gast.Paragraph, source []byte) string {
	return strings.TrimSpace(renderInlineChildren(node, source))
}

func renderHeading(node *gast.Heading, source []byte) string {
	return wrapInline("*", renderInlineChildren(node, source))
}

func indentMultiline(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for idx, line := range lines {
		lines[idx] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func renderListItem(node *gast.ListItem, ordered bool, index int, source []byte, depth int) string {
	marker := "•"
	if ordered {
		marker = fmt.Sprintf("%d.", index+1)
	}
	childBlocks := make([]string, 0)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		childBlocks = append(childBlocks, renderBlockContent(child, source, depth+1)...)
	}
	itemText := strings.TrimSpace(strings.Join(childBlocks, "\n"))
	if itemText == "" {
		itemText = "(empty)"
	}
	lines := strings.Split(itemText, "\n")
	indentation := strings.Repeat("  ", depth)
	continuation := indentation + "  "
	for idx, line := range lines {
		if idx == 0 {
			lines[idx] = indentation + marker + " " + line
			continue
		}
		lines[idx] = continuation + line
	}
	return strings.Join(lines, "\n")
}

func renderList(node *gast.List, source []byte, depth int) string {
	ordered := node.IsOrdered()
	out := make([]string, 0)
	start := 0
	if ordered && node.Start > 0 {
		start = node.Start - 1
	}
	index := start
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*gast.ListItem)
		if !ok {
			continue
		}
		out = append(out, renderListItem(item, ordered, index, source, depth))
		index++
	}
	return strings.Join(out, "\n")
}

func renderLines(lines gast.Node, source []byte) string {
	if lines == nil || lines.Lines().Len() == 0 {
		return ""
	}
	return string(lines.Lines().Value(source))
}

func renderCodeBlock(text string, language string) string {
	header := ""
	if strings.TrimSpace(language) != "" {
		header = strings.TrimSpace(language) + "\n"
	}
	return "```\n" + header + text + "\n```"
}

func renderTableCell(node *extast.TableCell, source []byte) string {
	value := strings.TrimSpace(renderInlineChildren(node, source))
	return strings.ReplaceAll(value, "\n", " ")
}

func renderTableRow(node gast.Node, source []byte) string {
	cells := make([]string, 0)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		cell, ok := child.(*extast.TableCell)
		if !ok {
			continue
		}
		cells = append(cells, renderTableCell(cell, source))
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func renderTable(node *extast.Table, source []byte) string {
	var rows []string
	var separator string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch typed := child.(type) {
		case *extast.TableHeader:
			for row := typed.FirstChild(); row != nil; row = row.NextSibling() {
				rows = append(rows, renderTableRow(row, source))
				if separator == "" {
					columns := 0
					for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
						if _, ok := cell.(*extast.TableCell); ok {
							columns++
						}
					}
					parts := make([]string, columns)
					for idx := range parts {
						parts[idx] = "---"
					}
					separator = "| " + strings.Join(parts, " | ") + " |"
				}
			}
		case *extast.TableRow:
			rows = append(rows, renderTableRow(typed, source))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows)+1)
	lines = append(lines, rows[0])
	if separator != "" {
		lines = append(lines, separator)
	}
	lines = append(lines, rows[1:]...)
	return renderCodeBlock(strings.Join(lines, "\n"), "")
}

func renderBlockquote(node *gast.Blockquote, source []byte, depth int) string {
	parts := make([]string, 0)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		parts = append(parts, renderBlockContent(child, source, depth)...)
	}
	rendered := strings.TrimSpace(strings.Join(parts, "\n"))
	if rendered == "" {
		return ""
	}
	return indentMultiline(rendered, "> ")
}

func renderBlockContent(node gast.Node, source []byte, depth int) []string {
	switch typed := node.(type) {
	case *gast.Paragraph:
		rendered := renderParagraph(typed, source)
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.TextBlock:
		rendered := strings.TrimSpace(renderInlineChildren(typed, source))
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.Heading:
		rendered := renderHeading(typed, source)
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.List:
		rendered := renderList(typed, source, depth)
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.Blockquote:
		rendered := renderBlockquote(typed, source, depth)
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.FencedCodeBlock:
		rendered := renderCodeBlock(renderLines(typed, source), string(typed.Language(source)))
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.CodeBlock:
		rendered := renderCodeBlock(renderLines(typed, source), "")
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *extast.Table:
		rendered := renderTable(typed, source)
		if rendered == "" {
			return nil
		}
		return []string{rendered}
	case *gast.ThematicBreak:
		return []string{"--------"}
	default:
		out := make([]string, 0)
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			out = append(out, renderBlockContent(child, source, depth)...)
		}
		return out
	}
}

func splitToMaxLength(text string, maxLength int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if len(trimmed) <= maxLength {
		return []string{trimmed}
	}
	lines := strings.Split(trimmed, "\n")
	chunks := make([]string, 0)
	current := ""
	for _, line := range lines {
		candidate := line
		if current != "" {
			candidate = current + "\n" + line
		}
		if len(candidate) <= maxLength {
			current = candidate
			continue
		}
		if current != "" {
			chunks = append(chunks, current)
			current = ""
		}
		if len(line) <= maxLength {
			current = line
			continue
		}
		for offset := 0; offset < len(line); offset += maxLength {
			end := offset + maxLength
			if end > len(line) {
				end = len(line)
			}
			chunks = append(chunks, line[offset:end])
		}
	}
	if current != "" {
		chunks = append(chunks, current)
	}
	return chunks
}

func toSectionBlock(text string) map[string]any {
	return map[string]any{
		"type": "section",
		"text": map[string]any{
			"type": "mrkdwn",
			"text": text,
		},
	}
}

func truncateFallbackText(text string) string {
	if len(text) <= maxFallbackTextLength {
		return text
	}
	return text[:maxFallbackTextLength] + "\n\n[truncated]"
}

func sectionsToMessages(sectionTexts []string) []renderedSlackMessage {
	if len(sectionTexts) == 0 {
		fallback := "I processed your request, but there was no renderable text output."
		return []renderedSlackMessage{{
			Text:   fallback,
			Blocks: []map[string]any{toSectionBlock(fallback)},
		}}
	}
	messages := make([]renderedSlackMessage, 0)
	current := make([]string, 0, maxBlocksPerMessage)
	flush := func() {
		if len(current) == 0 {
			return
		}
		text := truncateFallbackText(strings.Join(current, "\n\n"))
		blocks := make([]map[string]any, 0, len(current))
		for _, section := range current {
			blocks = append(blocks, toSectionBlock(section))
		}
		messages = append(messages, renderedSlackMessage{Text: text, Blocks: blocks})
		current = current[:0]
	}
	for _, section := range sectionTexts {
		if len(current) >= maxBlocksPerMessage {
			flush()
		}
		current = append(current, section)
	}
	flush()
	return messages
}

func flattenRenderedText(messages []renderedSlackMessage) string {
	var out bytes.Buffer
	for idx, message := range messages {
		if idx > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(strings.TrimSpace(message.Text))
	}
	return strings.TrimSpace(out.String())
}

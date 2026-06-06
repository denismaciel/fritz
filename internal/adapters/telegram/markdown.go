package telegram

import (
	"fmt"
	"html"
	"strings"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var telegramMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func markdownToTelegramHTML(markdown string) string {
	trimmed := strings.TrimSpace(markdown)
	if trimmed == "" {
		return ""
	}
	source := []byte(trimmed)
	doc := telegramMarkdown.Parser().Parse(text.NewReader(source))
	var blocks []string
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		rendered := strings.TrimSpace(renderTelegramBlock(node, source, 0))
		if rendered != "" {
			blocks = append(blocks, rendered)
		}
	}
	if len(blocks) == 0 {
		return escapeTelegramHTML(trimmed)
	}
	return strings.Join(blocks, "\n\n")
}

func escapeTelegramHTML(value string) string {
	return html.EscapeString(value)
}

func renderTelegramInlineChildren(parent gast.Node, source []byte) string {
	var out strings.Builder
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(renderTelegramInline(child, source))
	}
	return out.String()
}

func renderTelegramInline(node gast.Node, source []byte) string {
	switch typed := node.(type) {
	case *gast.Text:
		value := escapeTelegramHTML(string(typed.Segment.Value(source)))
		if typed.HardLineBreak() || typed.SoftLineBreak() {
			value += "\n"
		}
		return value
	case *gast.String:
		return escapeTelegramHTML(string(typed.Value))
	case *gast.CodeSpan:
		return "<code>" + escapeTelegramHTML(renderTelegramInlineChildren(typed, source)) + "</code>"
	case *gast.Emphasis:
		content := strings.TrimSpace(renderTelegramInlineChildren(typed, source))
		if content == "" {
			return ""
		}
		if typed.Level >= 2 {
			return "<b>" + content + "</b>"
		}
		return "<i>" + content + "</i>"
	case *extast.Strikethrough:
		content := strings.TrimSpace(renderTelegramInlineChildren(typed, source))
		if content == "" {
			return ""
		}
		return "<s>" + content + "</s>"
	case *gast.Link:
		label := strings.TrimSpace(renderTelegramInlineChildren(typed, source))
		url := escapeTelegramHTML(strings.TrimSpace(string(typed.Destination)))
		if label == "" {
			label = url
		}
		return `<a href="` + url + `">` + label + `</a>`
	case *gast.AutoLink:
		label := escapeTelegramHTML(strings.TrimSpace(string(typed.Label(source))))
		url := escapeTelegramHTML(strings.TrimSpace(string(typed.URL(source))))
		if label == "" {
			label = url
		}
		return `<a href="` + url + `">` + label + `</a>`
	default:
		return renderTelegramInlineChildren(node, source)
	}
}

func renderTelegramBlock(node gast.Node, source []byte, depth int) string {
	switch typed := node.(type) {
	case *gast.Paragraph:
		return strings.TrimSpace(renderTelegramInlineChildren(typed, source))
	case *gast.TextBlock:
		return strings.TrimSpace(renderTelegramInlineChildren(typed, source))
	case *gast.Heading:
		content := strings.TrimSpace(renderTelegramInlineChildren(typed, source))
		if content == "" {
			return ""
		}
		return "<b>" + content + "</b>"
	case *gast.List:
		return renderTelegramList(typed, source, depth)
	case *gast.ListItem:
		return renderTelegramListItem(typed, false, 0, source, depth)
	case *gast.FencedCodeBlock:
		return renderTelegramCodeBlock(string(typed.Lines().Value(source)), string(typed.Language(source)))
	case *gast.CodeBlock:
		return renderTelegramCodeBlock(string(typed.Lines().Value(source)), "")
	case *gast.Blockquote:
		return renderTelegramBlockquote(typed, source, depth)
	case *gast.ThematicBreak:
		return "----------"
	case *extast.Table:
		return renderTelegramTable(typed, source)
	default:
		var parts []string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if rendered := strings.TrimSpace(renderTelegramBlock(child, source, depth)); rendered != "" {
				parts = append(parts, rendered)
			}
		}
		return strings.Join(parts, "\n")
	}
}

func renderTelegramList(node *gast.List, source []byte, depth int) string {
	var items []string
	index := 0
	if node.IsOrdered() && node.Start > 0 {
		index = node.Start - 1
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*gast.ListItem)
		if !ok {
			continue
		}
		items = append(items, renderTelegramListItem(item, node.IsOrdered(), index, source, depth))
		index++
	}
	return strings.Join(items, "\n")
}

func renderTelegramListItem(node *gast.ListItem, ordered bool, index int, source []byte, depth int) string {
	var blocks []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if rendered := strings.TrimSpace(renderTelegramBlock(child, source, depth+1)); rendered != "" {
			blocks = append(blocks, rendered)
		}
	}
	body := strings.TrimSpace(strings.Join(blocks, "\n"))
	if body == "" {
		return ""
	}
	marker := "•"
	if ordered {
		marker = fmt.Sprintf("%d.", index+1)
	}
	indent := strings.Repeat("  ", depth)
	lines := strings.Split(body, "\n")
	for idx, line := range lines {
		if idx == 0 {
			lines[idx] = indent + marker + " " + line
			continue
		}
		lines[idx] = indent + "  " + line
	}
	return strings.Join(lines, "\n")
}

func renderTelegramCodeBlock(body string, language string) string {
	body = strings.TrimSuffix(body, "\n")
	if strings.TrimSpace(language) != "" {
		body = strings.TrimSpace(language) + "\n" + body
	}
	return "<pre>" + escapeTelegramHTML(body) + "</pre>"
}

func renderTelegramBlockquote(node *gast.Blockquote, source []byte, depth int) string {
	var parts []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if rendered := strings.TrimSpace(renderTelegramBlock(child, source, depth)); rendered != "" {
			parts = append(parts, rendered)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	for idx, line := range lines {
		lines[idx] = "&gt; " + line
	}
	return strings.Join(lines, "\n")
}

func renderTelegramTable(node *extast.Table, source []byte) string {
	var rows []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch typed := child.(type) {
		case *extast.TableHeader:
			for row := typed.FirstChild(); row != nil; row = row.NextSibling() {
				rows = append(rows, renderTelegramTableRow(row, source))
			}
		case *extast.TableRow:
			rows = append(rows, renderTelegramTableRow(typed, source))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	return "<pre>" + escapeTelegramHTML(strings.Join(rows, "\n")) + "</pre>"
}

func renderTelegramTableRow(node gast.Node, source []byte) string {
	var cells []string
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if cell, ok := child.(*extast.TableCell); ok {
			cells = append(cells, strings.TrimSpace(renderTelegramInlineChildren(cell, source)))
		}
	}
	return strings.Join(cells, " | ")
}

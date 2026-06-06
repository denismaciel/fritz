package telegram

import (
	"strings"
	"testing"
)

func TestMarkdownToTelegramHTMLRendersCommonMarkdown(t *testing.T) {
	rendered := markdownToTelegramHTML("### Title\n\n- **one**\n- `two`\n\n```go\nfmt.Println(\"<hi>\")\n```")
	for _, want := range []string{
		"<b>Title</b>",
		"• <b>one</b>",
		"• <code>two</code>",
		"<pre>go\nfmt.Println(&#34;&lt;hi&gt;&#34;)</pre>",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered missing %q:\n%s", want, rendered)
		}
	}
}


package tool

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

type Edit struct {
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
}

func PrepareEditArguments(input any) any {
	args, ok := input.(map[string]any)
	if !ok {
		return input
	}
	oldText, oldOK := args["oldText"].(string)
	newText, newOK := args["newText"].(string)
	if !oldOK || !newOK {
		return input
	}

	var edits []map[string]any
	if rawEdits, ok := args["edits"].([]map[string]any); ok {
		edits = append(edits, rawEdits...)
	} else if rawEdits, ok := args["edits"].([]any); ok {
		for _, raw := range rawEdits {
			if editMap, ok := raw.(map[string]any); ok {
				edits = append(edits, editMap)
			}
		}
	}
	edits = append(edits, map[string]any{"oldText": oldText, "newText": newText})

	out := map[string]any{}
	for key, value := range args {
		if key == "oldText" || key == "newText" {
			continue
		}
		out[key] = value
	}
	out["edits"] = edits
	return out
}

func DetectLineEnding(content string) string {
	crlfIdx := strings.Index(content, "\r\n")
	lfIdx := strings.Index(content, "\n")
	if lfIdx == -1 || (crlfIdx != -1 && crlfIdx < lfIdx) {
		return "\r\n"
	}
	return "\n"
}

func NormalizeToLF(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
}

func RestoreLineEndings(text string, ending string) string {
	if ending == "\r\n" {
		return strings.ReplaceAll(text, "\n", "\r\n")
	}
	return text
}

func StripBOM(content string) (string, string) {
	if strings.HasPrefix(content, "\ufeff") {
		return "\ufeff", strings.TrimPrefix(content, "\ufeff")
	}
	return "", content
}

func NormalizeForFuzzyMatch(text string) string {
	text = norm.NFKC.String(text)
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	text = strings.Join(lines, "\n")
	return strings.NewReplacer(
		"\u2018", "'",
		"\u2019", "'",
		"\u201a", "'",
		"\u201b", "'",
		"\u201c", "\"",
		"\u201d", "\"",
		"\u201e", "\"",
		"\u201f", "\"",
		"\u2010", "-",
		"\u2011", "-",
		"\u2012", "-",
		"\u2013", "-",
		"\u2014", "-",
		"\u2015", "-",
		"\u2212", "-",
		"\u00a0", " ",
		"\u2002", " ",
		"\u2003", " ",
		"\u2004", " ",
		"\u2005", " ",
		"\u2006", " ",
		"\u2007", " ",
		"\u2008", " ",
		"\u2009", " ",
		"\u200a", " ",
		"\u202f", " ",
		"\u205f", " ",
		"\u3000", " ",
	).Replace(text)
}

type fuzzyMatchResult struct {
	Found          bool
	Index          int
	MatchLength    int
	UsedFuzzyMatch bool
}

func fuzzyFindText(content string, oldText string) fuzzyMatchResult {
	if idx := strings.Index(content, oldText); idx != -1 {
		return fuzzyMatchResult{Found: true, Index: idx, MatchLength: len(oldText)}
	}
	fuzzyContent := NormalizeForFuzzyMatch(content)
	fuzzyOld := NormalizeForFuzzyMatch(oldText)
	idx := strings.Index(fuzzyContent, fuzzyOld)
	if idx == -1 {
		return fuzzyMatchResult{}
	}
	return fuzzyMatchResult{
		Found:          true,
		Index:          idx,
		MatchLength:    len(fuzzyOld),
		UsedFuzzyMatch: true,
	}
}

func countOccurrences(content string, oldText string) int {
	fuzzyContent := NormalizeForFuzzyMatch(content)
	fuzzyOld := NormalizeForFuzzyMatch(oldText)
	return strings.Count(fuzzyContent, fuzzyOld)
}

func ApplyEditsToNormalizedContent(normalizedContent string, edits []Edit, path string) (string, string, error) {
	if len(edits) == 0 {
		return "", "", fmt.Errorf("Edit tool input is invalid. edits must contain at least one replacement.")
	}

	type matchedEdit struct {
		EditIndex   int
		MatchIndex  int
		MatchLength int
		NewText     string
	}

	normalizedEdits := make([]Edit, 0, len(edits))
	baseContent := normalizedContent
	for i, edit := range edits {
		if edit.OldText == "" {
			if len(edits) == 1 {
				return "", "", fmt.Errorf("oldText must not be empty in %s.", path)
			}
			return "", "", fmt.Errorf("edits[%d].oldText must not be empty in %s.", i, path)
		}
		next := Edit{
			OldText: NormalizeToLF(edit.OldText),
			NewText: NormalizeToLF(edit.NewText),
		}
		normalizedEdits = append(normalizedEdits, next)
		if fuzzyFindText(normalizedContent, next.OldText).UsedFuzzyMatch {
			baseContent = NormalizeForFuzzyMatch(normalizedContent)
		}
	}

	matched := make([]matchedEdit, 0, len(normalizedEdits))
	for i, edit := range normalizedEdits {
		match := fuzzyFindText(baseContent, edit.OldText)
		if !match.Found {
			if len(normalizedEdits) == 1 {
				return "", "", fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
			}
			return "", "", fmt.Errorf("Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", i, path)
		}
		occurrences := countOccurrences(baseContent, edit.OldText)
		if occurrences > 1 {
			if len(normalizedEdits) == 1 {
				return "", "", fmt.Errorf("Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", occurrences, path)
			}
			return "", "", fmt.Errorf("Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", occurrences, i, path)
		}
		matched = append(matched, matchedEdit{
			EditIndex:   i,
			MatchIndex:  match.Index,
			MatchLength: match.MatchLength,
			NewText:     edit.NewText,
		})
	}

	slices.SortFunc(matched, func(a matchedEdit, b matchedEdit) int {
		return a.MatchIndex - b.MatchIndex
	})
	for i := 1; i < len(matched); i++ {
		prev := matched[i-1]
		curr := matched[i]
		if prev.MatchIndex+prev.MatchLength > curr.MatchIndex {
			return "", "", fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.", prev.EditIndex, curr.EditIndex, path)
		}
	}

	newContent := baseContent
	for i := len(matched) - 1; i >= 0; i-- {
		edit := matched[i]
		newContent = newContent[:edit.MatchIndex] + edit.NewText + newContent[edit.MatchIndex+edit.MatchLength:]
	}
	if newContent == baseContent {
		if len(normalizedEdits) == 1 {
			return "", "", fmt.Errorf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", path)
		}
		return "", "", fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
	}
	return baseContent, newContent, nil
}

func GenerateDiffString(before string, after string) (string, int) {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")
	firstChanged := 1

	maxLen := len(beforeLines)
	if len(afterLines) > maxLen {
		maxLen = len(afterLines)
	}
	for i := 0; i < maxLen; i++ {
		var beforeLine string
		if i < len(beforeLines) {
			beforeLine = beforeLines[i]
		}
		var afterLine string
		if i < len(afterLines) {
			afterLine = afterLines[i]
		}
		if beforeLine != afterLine {
			firstChanged = i + 1
			break
		}
	}

	var out []string
	gap := 0
	for i := 0; i < maxLen; i++ {
		var beforeLine string
		if i < len(beforeLines) {
			beforeLine = beforeLines[i]
		}
		var afterLine string
		if i < len(afterLines) {
			afterLine = afterLines[i]
		}
		if beforeLine == afterLine {
			gap++
			continue
		}
		if gap > 6 {
			out = append(out, fmt.Sprintf("... %d unchanged lines ...", gap))
		} else {
			for j := i - gap; j < i; j++ {
				if j >= 0 && j < len(afterLines) {
					out = append(out, " "+afterLines[j])
				}
			}
		}
		gap = 0
		if beforeLine != "" || i < len(beforeLines) {
			out = append(out, "-"+beforeLine)
		}
		if afterLine != "" || i < len(afterLines) {
			out = append(out, "+"+afterLine)
		}
	}
	return strings.Join(out, "\n"), firstChanged
}

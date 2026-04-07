package tool

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
)

const narrowNoBreakSpace = "\u202f"

var unicodeSpaceReplacer = strings.NewReplacer(
	"\u00a0", " ",
	"\u2000", " ",
	"\u2001", " ",
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
)

func ExpandPath(filePath string) string {
	normalized := unicodeSpaceReplacer.Replace(strings.TrimPrefix(filePath, "@"))
	if normalized == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return normalized
		}
		return home
	}
	if strings.HasPrefix(normalized, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return normalized
		}
		return filepath.Join(home, strings.TrimPrefix(normalized, "~/"))
	}
	return normalized
}

func ResolveToCwd(filePath string, cwd string) string {
	expanded := ExpandPath(filePath)
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded)
	}
	return filepath.Clean(filepath.Join(cwd, expanded))
}

func ResolveReadPath(filePath string, cwd string) string {
	resolved := ResolveToCwd(filePath, cwd)
	if fileExists(resolved) {
		return resolved
	}

	amPmVariant := strings.ReplaceAll(resolved, " AM.", narrowNoBreakSpace+"AM.")
	amPmVariant = strings.ReplaceAll(amPmVariant, " PM.", narrowNoBreakSpace+"PM.")
	if amPmVariant != resolved && fileExists(amPmVariant) {
		return amPmVariant
	}

	nfdVariant := norm.NFD.String(resolved)
	if nfdVariant != resolved && fileExists(nfdVariant) {
		return nfdVariant
	}

	curlyVariant := strings.ReplaceAll(resolved, "'", "\u2019")
	if curlyVariant != resolved && fileExists(curlyVariant) {
		return curlyVariant
	}

	nfdCurlyVariant := strings.ReplaceAll(nfdVariant, "'", "\u2019")
	if nfdCurlyVariant != resolved && fileExists(nfdCurlyVariant) {
		return nfdCurlyVariant
	}

	return resolved
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

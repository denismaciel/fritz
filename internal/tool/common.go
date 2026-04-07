package tool

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

func requireStringArg(root string, call Call, key string) (string, Result, error) {
	raw, ok := call.Args[key].(string)
	if !ok || raw == "" {
		err := fmt.Errorf("missing required arg: %s", key)
		return "", errorResult(call, err), err
	}
	return raw, Result{}, nil
}

func requirePathWithinRoot(root string, call Call, rawPath string) (string, Result, error) {
	resolved := ResolveToCwd(rawPath, root)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		err := fmt.Errorf("path %q escapes root", rawPath)
		return "", errorResult(call, err), err
	}
	return resolved, Result{}, nil
}

func errorResult(call Call, err error) Result {
	return ErrorTextResult(call, err)
}

func detectSupportedImageMimeType(data []byte) string {
	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return mimeType
	default:
		return ""
	}
}

func intArg(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func floatArg(value any) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float32:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
}

func boolArg(value any) bool {
	flag, _ := value.(bool)
	return flag
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

var errStopWalk = errors.New("stop walk")

func collectSearchFiles(ops FileOperations, path string) ([]string, error) {
	info, err := ops.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	err = ops.WalkDir(path, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if current != path && shouldIgnore(ops, current, path, true) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldIgnore(ops, current, path, false) {
			return nil
		}
		files = append(files, current)
		return nil
	})
	return files, err
}

func formatSearchPath(file string, root string, isDir bool) string {
	if !isDir {
		return filepath.Base(file)
	}
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return filepath.Base(file)
	}
	return filepath.ToSlash(rel)
}

func buildPatternMatcher(pattern string, literal bool, ignoreCase bool) (func(string) bool, error) {
	if literal {
		if ignoreCase {
			pattern = strings.ToLower(pattern)
			return func(line string) bool { return strings.Contains(strings.ToLower(line), pattern) }, nil
		}
		return func(line string) bool { return strings.Contains(line, pattern) }, nil
	}
	if ignoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return re.MatchString, nil
}

func shouldIgnore(ops FileOperations, path string, root string, isDir bool) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == ".git" || part == "node_modules" {
			return true
		}
	}
	ignored := false
	for _, rule := range loadGitignoreRules(ops, root, path) {
		if !rule.matches(path, root, isDir) {
			continue
		}
		ignored = !rule.Negated
	}
	return ignored
}

type gitignoreRule struct {
	BaseDir  string
	Pattern  string
	Negated  bool
	DirOnly  bool
	Anchored bool
}

func loadGitignoreRules(ops FileOperations, root string, path string) []gitignoreRule {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	dir := path
	if info, err := ops.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	var dirs []string
	for current := dir; ; current = filepath.Dir(current) {
		dirs = append(dirs, current)
		if current == root || current == "." || current == "/" || current == filepath.Dir(current) {
			break
		}
	}
	slices.Reverse(dirs)

	var rules []gitignoreRule
	for _, current := range dirs {
		data, err := ops.ReadFile(filepath.Join(current, ".gitignore"))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			rule := gitignoreRule{BaseDir: current}
			if strings.HasPrefix(line, "!") {
				rule.Negated = true
				line = strings.TrimPrefix(line, "!")
			}
			if strings.HasSuffix(line, "/") {
				rule.DirOnly = true
				line = strings.TrimSuffix(line, "/")
			}
			if strings.HasPrefix(line, "/") {
				rule.Anchored = true
				line = strings.TrimPrefix(line, "/")
			}
			rule.Pattern = filepath.ToSlash(line)
			rules = append(rules, rule)
		}
	}
	return rules
}

func globMatch(pattern string, value string) bool {
	pattern = filepath.ToSlash(pattern)
	value = filepath.ToSlash(value)
	return matchGlobSegments(splitGlob(pattern), splitGlob(value))
}

func (r gitignoreRule) matches(path string, root string, isDir bool) bool {
	if r.DirOnly && !isDir {
		return false
	}
	relToBase, err := filepath.Rel(r.BaseDir, path)
	if err != nil {
		return false
	}
	relToBase = filepath.ToSlash(relToBase)
	baseName := filepath.Base(relToBase)
	pattern := filepath.ToSlash(r.Pattern)

	if r.Anchored || strings.Contains(pattern, "/") {
		return globMatch(pattern, relToBase)
	}
	if globMatch(pattern, baseName) {
		return true
	}
	return globMatch("**/"+pattern, relToBase)
}

func splitGlob(value string) []string {
	value = strings.Trim(filepath.ToSlash(value), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func matchGlobSegments(pattern []string, value []string) bool {
	if len(pattern) == 0 {
		return len(value) == 0
	}
	if pattern[0] == "**" {
		if matchGlobSegments(pattern[1:], value) {
			return true
		}
		if len(value) == 0 {
			return false
		}
		return matchGlobSegments(pattern, value[1:])
	}
	if len(value) == 0 {
		return false
	}
	ok, err := filepath.Match(pattern[0], value[0])
	if err != nil || !ok {
		return false
	}
	return matchGlobSegments(pattern[1:], value[1:])
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

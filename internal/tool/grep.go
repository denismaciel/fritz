package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type grepRunOptions struct {
	pattern      string
	contextLines int
	limit        int
	ignoreCase   bool
	literal      bool
	glob         string
}

type rgMatch struct {
	filePath    string
	lineNumber  int
	lineText    string
	hasLineText bool
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}

func grepOptionsFromCall(call Call) grepRunOptions {
	limit := DefaultGrepLimit
	if _, ok := call.Args["limit"]; ok {
		limit = max(1, intArg(call.Args["limit"]))
	}
	return grepRunOptions{
		pattern:      stringValue(call.Args["pattern"]),
		contextLines: max(0, intArg(call.Args["context"])),
		limit:        limit,
		ignoreCase:   boolArg(call.Args["ignoreCase"]),
		literal:      boolArg(call.Args["literal"]),
		glob:         stringValue(call.Args["glob"]),
	}
}

func (t grepTool) runRipgrep(ctx context.Context, call Call, resolved string) (Result, error) {
	options := grepOptionsFromCall(call)
	rgPath := t.rgPath
	if rgPath == "" {
		found, err := exec.LookPath("rg")
		if err != nil {
			err := errors.New("ripgrep (rg) is not available")
			return errorResult(call, err), err
		}
		rgPath = found
	}
	info, err := t.ops.Stat(resolved)
	if err != nil {
		err := fmt.Errorf("Path not found: %s", resolved)
		return errorResult(call, err), err
	}
	isDir := info.IsDir()

	args := []string{"--json", "--line-number", "--color=never", "--hidden"}
	if options.ignoreCase {
		args = append(args, "--ignore-case")
	}
	if options.literal {
		args = append(args, "--fixed-strings")
	}
	if options.glob != "" {
		args = append(args, "--glob", options.glob)
	}
	args = append(args, "--", options.pattern, resolved)

	cmd := exec.CommandContext(ctx, rgPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errorResult(call, err), err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		err := fmt.Errorf("Failed to run ripgrep: %s", err.Error())
		return errorResult(call, err), err
	}

	var matches []rgMatch
	matchCount := 0
	matchLimitReached := false
	killedDueToLimit := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || matchCount >= options.limit {
			continue
		}
		var event rgEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Type != "match" {
			continue
		}
		matchCount++
		if event.Data.Path.Text != "" && event.Data.LineNumber > 0 {
			matches = append(matches, rgMatch{
				filePath:    event.Data.Path.Text,
				lineNumber:  event.Data.LineNumber,
				lineText:    event.Data.Lines.Text,
				hasLineText: true,
			})
		}
		if matchCount >= options.limit {
			matchLimitReached = true
			killedDueToLimit = true
			_ = cmd.Process.Kill()
		}
	}
	if scanErr := scanner.Err(); scanErr != nil && !killedDueToLimit {
		_ = cmd.Wait()
		return errorResult(call, scanErr), scanErr
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		err := errors.New("Operation aborted")
		return errorResult(call, err), err
	}
	if waitErr != nil && !killedDueToLimit {
		if exitErr, ok := waitErr.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return TextResult(call, "No matches found"), nil
		}
		errText := strings.TrimSpace(stderr.String())
		if errText == "" {
			errText = waitErr.Error()
		}
		err := errors.New(errText)
		return errorResult(call, err), err
	}
	if matchCount == 0 {
		return TextResult(call, "No matches found"), nil
	}

	return t.formatGrepMatches(call, resolved, isDir, options, matches, matchLimitReached)
}

func (t grepTool) runGoGrep(call Call, resolved string) (Result, error) {
	options := grepOptionsFromCall(call)
	files, err := collectSearchFiles(t.ops, resolved)
	if err != nil {
		return errorResult(call, err), err
	}
	isDir := true
	if info, statErr := t.ops.Stat(resolved); statErr == nil {
		isDir = info.IsDir()
	}
	matcher, err := buildPatternMatcher(options.pattern, options.literal, options.ignoreCase)
	if err != nil {
		return errorResult(call, err), err
	}

	var matches []rgMatch
	matchCount := 0
	matchLimitReached := false
	for _, file := range files {
		displayPath := formatSearchPath(file, resolved, isDir)
		if options.glob != "" && !globMatch(options.glob, displayPath) {
			continue
		}
		data, readErr := t.ops.ReadFile(file)
		if readErr != nil {
			continue
		}
		lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		for i, line := range lines {
			if !matcher(line) {
				continue
			}
			matchCount++
			matches = append(matches, rgMatch{
				filePath:    file,
				lineNumber:  i + 1,
				lineText:    line,
				hasLineText: true,
			})
			if matchCount >= options.limit {
				matchLimitReached = true
				break
			}
		}
		if matchLimitReached {
			break
		}
	}
	if matchCount == 0 {
		return TextResult(call, "No matches found"), nil
	}
	return t.formatGrepMatches(call, resolved, isDir, options, matches, matchLimitReached)
}

func (t grepTool) formatGrepMatches(call Call, resolved string, isDir bool, options grepRunOptions, matches []rgMatch, matchLimitReached bool) (Result, error) {
	var outputLines []string
	linesTruncated := false
	for _, match := range matches {
		if options.contextLines == 0 && match.hasLineText {
			line := strings.TrimSuffix(strings.ReplaceAll(strings.ReplaceAll(match.lineText, "\r\n", "\n"), "\r", ""), "\n")
			truncated, wasTruncated := TruncateLine(line, GrepMaxLineLength)
			linesTruncated = linesTruncated || wasTruncated
			outputLines = append(outputLines, fmt.Sprintf("%s:%d: %s", formatSearchPath(match.filePath, resolved, isDir), match.lineNumber, truncated))
			continue
		}
		block, blockTruncated := t.formatGrepContextBlock(match.filePath, resolved, isDir, match.lineNumber, options.contextLines)
		linesTruncated = linesTruncated || blockTruncated
		outputLines = append(outputLines, block...)
	}

	rawOutput := strings.Join(outputLines, "\n")
	truncation := TruncateHead(rawOutput, TruncationOptions{MaxLines: 1 << 20})
	output := truncation.Content
	var details *GrepResultDetails
	var notices []string
	if matchLimitReached {
		details = ensureGrepDetails(details)
		details.MatchLimitReached = options.limit
		notices = append(notices, fmt.Sprintf("%d matches limit reached. Use limit=%d for more, or refine pattern", options.limit, options.limit*2))
	}
	if truncation.Truncated {
		details = ensureGrepDetails(details)
		details.Truncation = &truncation
		notices = append(notices, fmt.Sprintf("%s limit reached", FormatSize(DefaultMaxBytes)))
	}
	if linesTruncated {
		details = ensureGrepDetails(details)
		details.LinesTruncated = true
		notices = append(notices, fmt.Sprintf("Some lines truncated to %d chars. Use read tool to see full lines", GrepMaxLineLength))
	}
	if len(notices) > 0 {
		output += "\n\n[" + strings.Join(notices, ". ") + "]"
	}
	var resultDetails any
	if details != nil {
		resultDetails = details
	}
	return Result{CallID: call.ID, Name: call.Name, Parts: []ContentPart{TextPart(output)}, Details: resultDetails}, nil
}

func (t grepTool) formatGrepContextBlock(filePath string, resolved string, isDir bool, lineNumber int, contextLines int) ([]string, bool) {
	displayPath := formatSearchPath(filePath, resolved, isDir)
	data, err := t.ops.ReadFile(filePath)
	if err != nil {
		return []string{fmt.Sprintf("%s:%d: (unable to read file)", displayPath, lineNumber)}, false
	}
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), "\n")
	start := lineNumber
	if contextLines > 0 {
		start = max(1, lineNumber-contextLines)
	}
	end := lineNumber
	if contextLines > 0 {
		end = min(len(lines), lineNumber+contextLines)
	}
	var block []string
	linesTruncated := false
	for current := start; current <= end; current++ {
		line := ""
		if current-1 >= 0 && current-1 < len(lines) {
			line = lines[current-1]
		}
		truncated, wasTruncated := TruncateLine(strings.ReplaceAll(line, "\r", ""), GrepMaxLineLength)
		linesTruncated = linesTruncated || wasTruncated
		if current == lineNumber {
			block = append(block, fmt.Sprintf("%s:%d: %s", displayPath, current, truncated))
		} else {
			block = append(block, fmt.Sprintf("%s-%d- %s", displayPath, current, truncated))
		}
	}
	return block, linesTruncated
}

func ensureGrepDetails(details *GrepResultDetails) *GrepResultDetails {
	if details != nil {
		return details
	}
	return &GrepResultDetails{}
}

func formatSearchPath(file string, root string, isDir bool) string {
	if !isDir {
		return filepath.Base(file)
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || rel == "" || strings.HasPrefix(rel, "..") {
		return filepath.Base(file)
	}
	return filepath.ToSlash(rel)
}

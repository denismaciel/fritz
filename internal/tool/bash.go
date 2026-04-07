package tool

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultBashOutputMaxBytes = 128 * 1024
)

type bashTool struct {
	root           string
	operations     BashOperations
	commandPrefix  string
	defaultTimeout time.Duration
	outputMaxBytes int
}

type BashExecOptions struct {
	Timeout    time.Duration
	MaxBytes   int
	SpillDir   string
	OnOutput   func(string) error
	LineBuffer bool
}

type BashExecResult struct {
	Output         string
	ExitCode       int
	Cancelled      bool
	TimedOut       bool
	Truncated      bool
	FullOutputPath string
}

type BashOperations interface {
	Exec(ctx context.Context, command string, cwd string, options BashExecOptions) (BashExecResult, error)
}

type localBashOperations struct{}

type BashToolOption func(*bashTool)

func WithCommandPrefix(prefix string) BashToolOption {
	return func(t *bashTool) {
		t.commandPrefix = prefix
	}
}

func WithBashOperations(operations BashOperations) BashToolOption {
	return func(t *bashTool) {
		t.operations = operations
	}
}

func WithDefaultTimeout(timeout time.Duration) BashToolOption {
	return func(t *bashTool) {
		t.defaultTimeout = timeout
	}
}

func WithOutputMaxBytes(maxBytes int) BashToolOption {
	return func(t *bashTool) {
		t.outputMaxBytes = maxBytes
	}
}

func CreateLocalBashOperations() BashOperations {
	return localBashOperations{}
}

func ExecuteBash(ctx context.Context, command string, cwd string, options BashExecOptions) (BashExecResult, error) {
	return localBashOperations{}.Exec(ctx, command, cwd, options)
}

func NewBashTool(root string, options ...BashToolOption) Tool {
	tool := bashTool{
		root:           root,
		operations:     CreateLocalBashOperations(),
		defaultTimeout: 30 * time.Second,
		outputMaxBytes: DefaultBashOutputMaxBytes,
	}
	for _, option := range options {
		option(&tool)
	}
	return tool
}

func (t bashTool) Definition() Definition {
	return Definition{
		Name:          "bash",
		Description:   "Run a shell command in the current working directory.",
		PromptSnippet: "Execute shell commands",
		PromptGuidelines: []string{
			"Prefer rg over grep in shell commands when available.",
		},
		Parameters: Parameters{
			Type: "object",
			Properties: map[string]Property{
				"command": {Type: "string", Description: "Shell command"},
				"timeout": {Type: "number", Description: "Timeout in seconds"},
			},
			Required: []string{"command"},
		},
	}
}

func (t bashTool) Run(ctx context.Context, call Call) (Result, error) {
	command, ok := call.Args["command"].(string)
	if !ok || strings.TrimSpace(command) == "" {
		err := errors.New("missing required arg: command")
		return errorResult(call, err), err
	}

	timeout := t.defaultTimeout
	if timeoutSeconds := floatArg(call.Args["timeout"]); timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds * float64(time.Second))
	}

	if t.commandPrefix != "" {
		command = t.commandPrefix + "\n" + command
	}

	execResult, err := t.operations.Exec(ctx, command, t.root, BashExecOptions{
		Timeout:  timeout,
		MaxBytes: t.outputMaxBytes,
		SpillDir: t.root,
	})
	if err != nil {
		result := ErrorTextResult(call, err)
		if execResult.Output != "" {
			result.Parts = []ContentPart{TextPart(strings.TrimRight(execResult.Output, "\n"))}
		}
		result.Details = BashResultDetails{
			ExitCode:       execResult.ExitCode,
			TimedOut:       execResult.TimedOut,
			Cancelled:      execResult.Cancelled,
			Truncated:      execResult.Truncated,
			FullOutputPath: execResult.FullOutputPath,
		}
		return result, err
	}

	content := strings.TrimRight(execResult.Output, "\n")
	if content == "" {
		content = "(no output)"
	}
	return Result{
		CallID: call.ID,
		Name:   call.Name,
		Parts:  []ContentPart{TextPart(content)},
		Details: BashResultDetails{
			ExitCode:       execResult.ExitCode,
			TimedOut:       execResult.TimedOut,
			Cancelled:      execResult.Cancelled,
			Truncated:      execResult.Truncated,
			FullOutputPath: execResult.FullOutputPath,
		},
	}, nil
}

func (o localBashOperations) Exec(ctx context.Context, command string, cwd string, options BashExecOptions) (BashExecResult, error) {
	if _, err := os.Stat(cwd); err != nil {
		return BashExecResult{}, fmt.Errorf("working directory does not exist: %s", cwd)
	}

	timeout := options.Timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return BashExecResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return BashExecResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return BashExecResult{}, err
	}

	sink, err := newOutputSink(options.MaxBytes, options.SpillDir)
	if err != nil {
		return BashExecResult{}, err
	}
	defer sink.Close()

	stream := func(r io.Reader) error {
		reader := bufio.NewReader(r)
		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				if err := sink.Write(chunk); err != nil {
					return err
				}
				if options.OnOutput != nil {
					if err := options.OnOutput(chunk); err != nil {
						return err
					}
				}
			}
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, os.ErrClosed) {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs <- stream(stdout)
	}()
	go func() {
		defer wg.Done()
		errs <- stream(stderr)
	}()

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- cmd.Wait()
	}()

	var waitErr error
	select {
	case waitErr = <-waitErrCh:
	case <-ctx.Done():
		_ = killProcessGroup(cmd.Process)
		waitErr = <-waitErrCh
	}

	wg.Wait()
	close(errs)
	for streamErr := range errs {
		if streamErr != nil {
			return BashExecResult{}, streamErr
		}
	}

	result := BashExecResult{
		Output:         sink.Output(),
		ExitCode:       0,
		Truncated:      sink.Truncated(),
		FullOutputPath: sink.FullOutputPath(),
	}

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.ExitCode = -1
		result.TimedOut = true
		return result, fmt.Errorf("command timed out after %d seconds", int(timeout/time.Second))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.ExitCode = -1
		result.Cancelled = true
		return result, context.Canceled
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, waitErr
	}
	return result, nil
}

func killProcessGroup(process *os.Process) error {
	if process == nil {
		return nil
	}
	return syscall.Kill(-process.Pid, syscall.SIGKILL)
}

type outputSink struct {
	maxBytes int
	spillDir string
	buf      strings.Builder
	file     *os.File
	bytes    int
	mu       sync.Mutex
}

func newOutputSink(maxBytes int, spillDir string) (*outputSink, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultBashOutputMaxBytes
	}
	if spillDir == "" {
		spillDir = os.TempDir()
	}
	return &outputSink{maxBytes: maxBytes, spillDir: spillDir}, nil
}

func (s *outputSink) Write(chunk string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data := []byte(chunk)
	if s.file == nil && s.bytes+len(data) > s.maxBytes {
		file, err := os.CreateTemp(s.spillDir, "fritz-bash-*")
		if err != nil {
			return err
		}
		if _, err := file.WriteString(s.buf.String()); err != nil {
			_ = file.Close()
			return err
		}
		s.file = file
	}
	if s.file != nil {
		if _, err := s.file.Write(data); err != nil {
			return err
		}
	}
	if s.bytes < s.maxBytes {
		remaining := s.maxBytes - s.bytes
		if len(data) > remaining {
			data = data[:remaining]
		}
		s.buf.Write(data)
	}
	s.bytes += len([]byte(chunk))
	return nil
}

func (s *outputSink) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	output := s.buf.String()
	if s.file != nil {
		output = strings.TrimRight(output, "\n") + "\n\n[output truncated. full output saved to " + s.file.Name() + "]"
	}
	return output
}

func (s *outputSink) Truncated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.file != nil
}

func (s *outputSink) FullOutputPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return ""
	}
	return s.file.Name()
}

func (s *outputSink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

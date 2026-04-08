package tmux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maestro/cmd"
	"maestro/log"
	"maestro/pkg/programs"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

const ProgramClaude = "claude"

const ProgramAider = "aider"
const ProgramGemini = "gemini"
const ProgramCodex = "codex"

// TmuxSession represents a managed tmux session
type TmuxSession struct {
	// Initialized by NewTmuxSession
	//
	// The name of the tmux session and the sanitized name used for tmux commands.
	sanitizedName string
	program       string
	// ptyFactory is used to create a PTY for the tmux session.
	ptyFactory PtyFactory
	// cmdExec is used to execute commands in the tmux session.
	cmdExec cmd.Executor

	// Initialized by Start or Restore
	//
	// ptmx is a PTY is running the tmux attach command. This can be resized to change the
	// stdout dimensions of the tmux pane. On detach, we close it and set a new one.
	// ptmxMu guards all accesses to ptmx across goroutines.
	ptmxMu sync.RWMutex
	ptmx   *os.File
	// monitor monitors the tmux pane content and sends signals to the UI when it's status changes
	monitor *statusMonitor

	// Initialized by Attach
	// Deinitilaized by Detach
	//
	// Channel to be closed at the very end of detaching. Used to signal callers.
	attachCh chan struct{}
	// While attached, we use some goroutines to manage the window size and stdin/stdout. This stuff
	// is used to terminate them on Detach. We don't want them to outlive the attached window.
	ctx    context.Context
	cancel func()
	wg     *sync.WaitGroup
}

const TmuxPrefix = "maestro_"

// tmuxMaxNameLen is the maximum length of a tmux session name.
// tmux enforces a hard limit of 256 characters; we stay well under it.
const tmuxMaxNameLen = 240

var whiteSpaceRegex = regexp.MustCompile(`\s+`)

// tmuxSpecialCharsRegex matches characters that are special in tmux target syntax.
// These include: . (window separator in some contexts), : (window/pane separator),
// { } (target modifiers), $ (session sigil), # (format string prefix),
// % (pane sigil), ' " (quoting).
var tmuxSpecialCharsRegex = regexp.MustCompile(`[.:{}\$#%'"]+`)

func toMaestroTmuxName(str string) string {
	str = whiteSpaceRegex.ReplaceAllString(str, "")
	str = tmuxSpecialCharsRegex.ReplaceAllString(str, "_")
	name := fmt.Sprintf("%s%s", TmuxPrefix, str)
	// Truncate to avoid exceeding tmux's 256-character session-name limit.
	if len(name) > tmuxMaxNameLen {
		// Preserve the prefix; hash the suffix to keep names unique.
		h := sha256.Sum256([]byte(str))
		suffix := fmt.Sprintf("%x", h[:4]) // 8 hex chars
		name = fmt.Sprintf("%s%s_%s", TmuxPrefix, str[:tmuxMaxNameLen-len(TmuxPrefix)-9], suffix)
	}
	return name
}

// SanitizeTmuxName applies the same sanitization used for tmux session names.
func SanitizeTmuxName(name string) string {
	return toMaestroTmuxName(name)
}

// NewTmuxSession creates a new TmuxSession with the given name and program.
func NewTmuxSession(name string, program string) *TmuxSession {
	return newTmuxSession(name, program, MakePtyFactory(), cmd.MakeExecutor())
}

// NewTmuxSessionWithDeps creates a new TmuxSession with provided dependencies for testing.
func NewTmuxSessionWithDeps(name string, program string, ptyFactory PtyFactory, cmdExec cmd.Executor) *TmuxSession {
	return newTmuxSession(name, program, ptyFactory, cmdExec)
}

func newTmuxSession(name string, program string, ptyFactory PtyFactory, cmdExec cmd.Executor) *TmuxSession {
	return &TmuxSession{
		sanitizedName: toMaestroTmuxName(name),
		program:       program,
		ptyFactory:    ptyFactory,
		cmdExec:       cmdExec,
	}
}

// Start creates and starts a new tmux session, then attaches to it. Program is the command to run in
// the session (ex. claude). workdir is the git worktree directory.
func (t *TmuxSession) Start(workDir string) error {
	return t.start(workDir, nil)
}

// StartWithEnv creates and starts a new tmux session with environment variables injected.
// The env map is passed to tmux via -e flags (requires tmux 3.2+).
func (t *TmuxSession) StartWithEnv(workDir string, env map[string]string) error {
	extraArgs := make([]string, 0, len(env)*2)
	for k, v := range env {
		extraArgs = append(extraArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	return t.start(workDir, extraArgs)
}

func (t *TmuxSession) start(workDir string, extraArgs []string) error {
	if t.DoesSessionExist() {
		return fmt.Errorf("tmux session already exists: %s", t.sanitizedName)
	}

	args := []string{"new-session"}
	args = append(args, extraArgs...)
	args = append(args, "-d", "-s", t.sanitizedName, "-c", workDir, t.program)

	cmd := exec.Command("tmux", args...)
	ptmx, err := t.ptyFactory.Start(cmd)
	if err != nil {
		if t.DoesSessionExist() {
			cleanupCmd := exec.Command("tmux", "kill-session", "-t", t.sanitizedName)
			if cleanupErr := t.cmdExec.Run(cleanupCmd); cleanupErr != nil {
				err = fmt.Errorf("%w (cleanup error: %v)", err, cleanupErr)
			}
		}
		return fmt.Errorf("error starting tmux session: %w", err)
	}

	timeout := time.After(2 * time.Second)
	sleepDuration := 5 * time.Millisecond
	for !t.DoesSessionExist() {
		select {
		case <-timeout:
			timeoutErr := fmt.Errorf("timed out waiting for tmux session %s to appear", t.sanitizedName)
			if cleanupErr := t.Close(); cleanupErr != nil {
				timeoutErr = fmt.Errorf("%w (cleanup error: %v)", timeoutErr, cleanupErr)
			}
			return timeoutErr
		default:
			time.Sleep(sleepDuration)
			if sleepDuration < 50*time.Millisecond {
				sleepDuration *= 2
			}
		}
	}
	ptmx.Close()

	historyCmd := exec.Command("tmux", "set-option", "-t", t.sanitizedName, "history-limit", "10000")
	if err := t.cmdExec.Run(historyCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to set history-limit for session %s: %v", t.sanitizedName, err)
	}

	mouseCmd := exec.Command("tmux", "set-option", "-t", t.sanitizedName, "mouse", "on")
	if err := t.cmdExec.Run(mouseCmd); err != nil {
		log.InfoLog.Printf("Warning: failed to enable mouse scrolling for session %s: %v", t.sanitizedName, err)
	}

	err = t.Restore()
	if err != nil {
		if cleanupErr := t.Close(); cleanupErr != nil {
			err = fmt.Errorf("%w (cleanup error: %v)", err, cleanupErr)
		}
		return fmt.Errorf("error restoring tmux session: %w", err)
	}

	return nil
}

// CheckAndHandleTrustPrompt checks the pane content once for a trust prompt and dismisses it if found.
// Returns true if the prompt was found and handled.
func (t *TmuxSession) CheckAndHandleTrustPrompt() bool {
	content, err := t.CapturePaneContent()
	if err != nil {
		return false
	}

	// Check for trust/permissions prompts using ProgramSpec registry first.
	spec, found := programs.GetByBinary(t.program)
	if found && len(spec.TrustPromptStrings) > 0 {
		for _, tps := range spec.TrustPromptStrings {
			if strings.Contains(content, tps) {
				if err := t.TapEnter(); err != nil {
					log.ErrorLog.Printf("could not tap enter on trust/MCP screen: %v", err)
				}
				return true
			}
		}
	} else {
		// Fallback to existing hardcoded checks
		if strings.HasSuffix(t.program, ProgramClaude) {
			if strings.Contains(content, "Do you trust the files in this folder?") ||
				strings.Contains(content, "new MCP server") {
				if err := t.TapEnter(); err != nil {
					log.ErrorLog.Printf("could not tap enter on trust/MCP screen: %v", err)
				}
				return true
			}
		} else {
			if strings.Contains(content, "Open documentation url for more info") {
				if err := t.TapDAndEnter(); err != nil {
					log.ErrorLog.Printf("could not tap enter on trust screen: %v", err)
				}
				return true
			}
		}
	}
	return false
}

// isTmuxServerRunning checks whether the tmux server is reachable using the
// session's cmdExec (so tests using a mock executor bypass the real tmux binary).
// Returns false and a descriptive error when the server is definitely not running
// or when tmux is not installed.
func (t *TmuxSession) isTmuxServerRunning() (bool, error) {
	// Fast path: verify tmux is in PATH before attempting to run it.
	if _, pathErr := exec.LookPath("tmux"); pathErr != nil {
		return false, fmt.Errorf("tmux is not installed or not in PATH: %w", pathErr)
	}

	// tmux writes "no server running on …" to stderr, so we must use
	// CombinedOutput (via exec.ExitError.Stderr) to capture it.
	listCmd := exec.Command("tmux", "list-sessions")
	out, err := t.cmdExec.Output(listCmd)
	if err == nil {
		return true, nil
	}
	// cmd.Output() only captures stdout; on failure the stderr is stored in
	// exec.ExitError.Stderr when the command was run without an explicit
	// Stderr sink — pull it out so the check below works.
	combined := strings.ToLower(string(out))
	if exitErr, ok := err.(*exec.ExitError); ok {
		combined += strings.ToLower(string(exitErr.Stderr))
	}
	if strings.Contains(combined, "no server running") ||
		strings.Contains(combined, "failed to connect to server") ||
		strings.Contains(combined, "error connecting to") {
		return false, fmt.Errorf("tmux server is not running; start a tmux server or restart the terminal")
	}
	// Any other error (e.g., server running but no sessions) is fine — the
	// server is alive, just no sessions currently.
	return true, nil
}

// Restore attaches to an existing session and restores the window size
func (t *TmuxSession) Restore() error {
	if running, err := t.isTmuxServerRunning(); !running {
		return err
	}
	ptmx, err := t.ptyFactory.Start(exec.Command("tmux", "attach-session", "-t", t.sanitizedName))
	if err != nil {
		return fmt.Errorf("error opening PTY: %w", err)
	}
	t.ptmxMu.Lock()
	t.ptmx = ptmx
	t.ptmxMu.Unlock()
	t.monitor = newStatusMonitor()
	return nil
}

type statusMonitor struct {
	// Store hashes to save memory.
	prevOutputHash []byte
}

func newStatusMonitor() *statusMonitor {
	return &statusMonitor{}
}

// hash hashes the string.
func (m *statusMonitor) hash(s string) []byte {
	h := sha256.New()
	_, _ = io.WriteString(h, s)
	return h.Sum(nil)
}

// TapEnter sends an enter keystroke to the tmux pane.
func (t *TmuxSession) TapEnter() error {
	t.ptmxMu.RLock()
	ptmx := t.ptmx
	t.ptmxMu.RUnlock()
	if ptmx == nil {
		return fmt.Errorf("PTY is not available (session detached)")
	}
	_, err := ptmx.Write([]byte{0x0D})
	if err != nil {
		return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
	}
	return nil
}

// TapDAndEnter sends 'D' followed by an enter keystroke to the tmux pane.
func (t *TmuxSession) TapDAndEnter() error {
	t.ptmxMu.RLock()
	ptmx := t.ptmx
	t.ptmxMu.RUnlock()
	if ptmx == nil {
		return fmt.Errorf("PTY is not available (session detached)")
	}
	_, err := ptmx.Write([]byte{0x44, 0x0D})
	if err != nil {
		return fmt.Errorf("error sending enter keystroke to PTY: %w", err)
	}
	return nil
}

func (t *TmuxSession) SendKeys(keys string) error {
	t.ptmxMu.RLock()
	ptmx := t.ptmx
	t.ptmxMu.RUnlock()
	if ptmx == nil {
		return fmt.Errorf("PTY is not available (session detached)")
	}
	if _, err := ptmx.Write([]byte(keys)); err != nil {
		return fmt.Errorf("error writing keys to PTY: %w", err)
	}
	return nil
}

// HasUpdated checks if the tmux pane content has changed since the last tick. It also returns true if
// the tmux pane has a prompt for aider or claude code.
func (t *TmuxSession) HasUpdated() (updated bool, hasPrompt bool) {
	if t.monitor == nil {
		return false, false
	}

	t.ptmxMu.RLock()
	ptmx := t.ptmx
	t.ptmxMu.RUnlock()
	if ptmx == nil {
		return false, false
	}

	content, err := t.CapturePaneContent()
	if err != nil {
		log.ErrorLog.Printf("error capturing pane content in status monitor: %v", err)
		return false, false
	}

	// Check for prompt strings using ProgramSpec registry first, with hardcoded fallback.
	spec, found := programs.GetByBinary(t.program)
	if found && len(spec.PromptStrings) > 0 {
		for _, ps := range spec.PromptStrings {
			if strings.Contains(content, ps) {
				hasPrompt = true
				break
			}
		}
	} else {
		// Fallback to existing hardcoded checks for programs not in the spec registry
		if strings.HasSuffix(t.program, ProgramClaude) {
			hasPrompt = strings.Contains(content, "No, and tell Claude what to do differently")
		} else if strings.HasPrefix(t.program, ProgramAider) {
			hasPrompt = strings.Contains(content, "(Y)es/(N)o/(D)on't ask again")
		} else if strings.HasPrefix(t.program, ProgramGemini) {
			hasPrompt = strings.Contains(content, "Yes, allow once")
		}
	}

	h := t.monitor.hash(content)
	if !bytes.Equal(h, t.monitor.prevOutputHash) {
		t.monitor.prevOutputHash = h
		return true, hasPrompt
	}
	return false, hasPrompt
}

func (t *TmuxSession) Attach() (chan struct{}, error) {
	if t.attachCh != nil {
		return nil, fmt.Errorf("session is already attached; call Detach first")
	}
	t.attachCh = make(chan struct{})

	t.wg = &sync.WaitGroup{}
	t.wg.Add(1)
	t.ctx, t.cancel = context.WithCancel(context.Background())

	// The first goroutine should terminate when the ptmx is closed. We use the
	// waitgroup to wait for it to finish.
	// The 2nd one returns when you press escape to Detach. It doesn't need to be
	// in the waitgroup because is the goroutine doing the Detaching; it waits for
	// all the other ones.
	t.ptmxMu.RLock()
	attachPtmx := t.ptmx
	t.ptmxMu.RUnlock()
	go func() {
		defer t.wg.Done()
		_, _ = io.Copy(os.Stdout, attachPtmx)
		// When io.Copy returns, it means the connection was closed
		// This could be due to normal detach or Ctrl-D
		// Check if the context is done to determine if it was a normal detach
		select {
		case <-t.ctx.Done():
			// Normal detach, do nothing
		default:
			// If context is not done, it was likely an abnormal termination (Ctrl-D)
			// Print warning message
			fmt.Fprintf(os.Stderr, "\n\033[31mError: Session terminated without detaching. Use Ctrl-Q to properly detach from tmux sessions.\033[0m\n")
		}
	}()

	go func() {
		// Close the channel after 50ms
		timeoutCh := make(chan struct{})
		go func() {
			time.Sleep(50 * time.Millisecond)
			close(timeoutCh)
		}()

		// Read input from stdin and check for Ctrl+q
		buf := make([]byte, 32)
		for {
			nr, err := os.Stdin.Read(buf)
			if err != nil {
				if err == io.EOF {
					break
				}
				continue
			}

			// Nuke the first bytes of stdin, up to 64, to prevent tmux from reading it.
			// When we attach, there tends to be terminal control sequences like ?[?62c0;95;0c or
			// ]10;rgb:f8f8f8. The control sequences depend on the terminal (warp vs iterm). We should use regex ideally
			// but this works well for now. Log this for debugging.
			//
			// There seems to always be control characters, but I think it's possible for there not to be. The heuristic
			// here can be: if there's characters within 50ms, then assume they are control characters and nuke them.
			select {
			case <-timeoutCh:
			default:
				log.InfoLog.Printf("nuked first stdin: %s", buf[:nr])
				continue
			}

			// Check for Ctrl+q (ASCII 17)
			if nr == 1 && buf[0] == 17 {
				// Detach from the session
				t.Detach()
				return
			}

			// Forward other input to tmux
			_, _ = attachPtmx.Write(buf[:nr])
		}
	}()

	t.monitorWindowSize()
	return t.attachCh, nil
}

// DetachSafely disconnects from the current tmux session without panicking
func (t *TmuxSession) DetachSafely() error {
	// Only detach if we're actually attached
	if t.attachCh == nil {
		return nil // Already detached
	}

	var errs []error

	// Close the attached pty session.
	t.ptmxMu.Lock()
	if t.ptmx != nil {
		if err := t.ptmx.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing attach pty session: %w", err))
		}
		t.ptmx = nil
	}
	t.ptmxMu.Unlock()

	// Clean up attach state
	if t.attachCh != nil {
		close(t.attachCh)
		t.attachCh = nil
	}

	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}

	if t.wg != nil {
		t.wg.Wait()
		t.wg = nil
	}

	t.ctx = nil

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Detach disconnects from the current tmux session. It panics if detaching fails. At the moment, there's no
// way to recover from a failed detach.
func (t *TmuxSession) Detach() {
	if t.attachCh == nil {
		return
	}

	// Capture attach-state fields before the defer clears them.
	attachCh := t.attachCh
	cancelFn := t.cancel
	wg := t.wg

	defer func() {
		close(attachCh)
		t.attachCh = nil
		t.cancel = nil
		t.ctx = nil
		t.wg = nil
	}()

	// Step 1: Close the attached pty session. This unblocks the stdout goroutine
	// (io.Copy returns on EOF) but the goroutine may still be running.
	t.ptmxMu.Lock()
	ptmxToClose := t.ptmx
	t.ptmx = nil
	t.ptmxMu.Unlock()
	if ptmxToClose == nil {
		// ptmx was already closed (e.g. by a concurrent DetachSafely call).
		// Nothing to close; proceed to restore below.
		msg := "Detach called with nil ptmx — session may have been concurrently detached"
		log.ErrorLog.Println(msg)
	} else if err := ptmxToClose.Close(); err != nil {
		// This is a fatal error. We can't detach if we can't close the PTY. It's better to just panic and have the
		// user re-invoke the program than to ruin their terminal pane.
		msg := fmt.Sprintf("error closing attach pty session: %v", err)
		log.ErrorLog.Println(msg)
		panic(msg)
	}

	// Step 2: Cancel goroutines created by Attach and wait for them to fully
	// exit before we call Restore(). This prevents the stdin goroutine from
	// writing buffered bytes into the new ptmx that Restore() assigns.
	cancelFn()
	wg.Wait()

	// Step 3: Attach goroutines have exited; safe to create a fresh PTY
	// attachment to the background session.
	if err := t.Restore(); err != nil {
		// This is a fatal error. Our invariant that a started TmuxSession always has a valid ptmx is violated.
		msg := fmt.Sprintf("error restoring tmux session after detach: %v", err)
		log.ErrorLog.Println(msg)
		panic(msg)
	}
}

// Close terminates the tmux session and cleans up resources
func (t *TmuxSession) Close() error {
	var errs []error

	t.ptmxMu.Lock()
	ptmxToClose := t.ptmx
	t.ptmx = nil
	t.ptmxMu.Unlock()
	if ptmxToClose != nil {
		if err := ptmxToClose.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PTY: %w", err))
		}
	}

	cmd := exec.Command("tmux", "kill-session", "-t", t.sanitizedName)
	if err := t.cmdExec.Run(cmd); err != nil {
		errs = append(errs, fmt.Errorf("error killing tmux session: %w", err))
	}

	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple errors occurred during cleanup:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return errors.New(errMsg)
}

// SetDetachedSize set the width and height of the session while detached. This makes the
// tmux output conform to the specified shape.
func (t *TmuxSession) SetDetachedSize(width, height int) error {
	return t.updateWindowSize(width, height)
}

// updateWindowSize updates the window size of the PTY.
func (t *TmuxSession) updateWindowSize(cols, rows int) error {
	t.ptmxMu.RLock()
	ptmx := t.ptmx
	t.ptmxMu.RUnlock()
	if ptmx == nil {
		return nil
	}
	return pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
		X:    0,
		Y:    0,
	})
}

func (t *TmuxSession) DoesSessionExist() bool {
	// Using "-t name" does a prefix match, which is wrong. `-t=` does an exact match.
	existsCmd := exec.Command("tmux", "has-session", fmt.Sprintf("-t=%s", t.sanitizedName))
	return t.cmdExec.Run(existsCmd) == nil
}

// CapturePaneContent captures the content of the tmux pane
func (t *TmuxSession) CapturePaneContent() (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("error capturing pane content: %w", err)
	}
	return string(output), nil
}

// CapturePaneContentWithOptions captures the pane content with additional options
// start and end specify the starting and ending line numbers (use "-" for the start/end of history)
func (t *TmuxSession) CapturePaneContentWithOptions(start, end string) (string, error) {
	// Add -e flag to preserve escape sequences (ANSI color codes)
	cmd := exec.Command("tmux", "capture-pane", "-p", "-e", "-J", "-S", start, "-E", end, "-t", t.sanitizedName)
	output, err := t.cmdExec.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to capture tmux pane content with options: %w", err)
	}
	return string(output), nil
}

// CleanupSessions kills all tmux sessions that start with "session-"
func CleanupSessions(cmdExec cmd.Executor) error {
	// First try to list sessions
	cmd := exec.Command("tmux", "ls")
	output, err := cmdExec.Output(cmd)

	// If there's an error and it's because no server is running, that's fine
	// Exit code 1 typically means no sessions exist
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil // No sessions to clean up
		}
		return fmt.Errorf("failed to list tmux sessions: %w", err)
	}

	re := regexp.MustCompile(fmt.Sprintf(`%s.*:`, TmuxPrefix))
	matches := re.FindAllString(string(output), -1)
	for i, match := range matches {
		// The regex always ends with ":" so Index should always find it, but
		// guard defensively to avoid a slice-bounds panic.
		if idx := strings.Index(match, ":"); idx >= 0 {
			matches[i] = match[:idx]
		}
	}

	for _, match := range matches {
		log.InfoLog.Printf("cleaning up session: %s", match)
		if err := cmdExec.Run(exec.Command("tmux", "kill-session", "-t", match)); err != nil {
			return fmt.Errorf("failed to kill tmux session %s: %w", match, err)
		}
	}
	return nil
}

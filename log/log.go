package log

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

var (
	WarningLog *log.Logger
	InfoLog    *log.Logger
	ErrorLog   *log.Logger
)

var logFileName = filepath.Join(os.TempDir(), "maestro.log")

var globalLogFile *os.File

func init() {
	// Pre-initialize loggers to stderr so callers (e.g. config.DefaultConfig)
	// never dereference a nil logger before Initialize() is called.
	WarningLog = log.New(os.Stderr, "WARNING:", log.Ldate|log.Ltime|log.Lshortfile)
	InfoLog = log.New(os.Stderr, "INFO:", log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(os.Stderr, "ERROR:", log.Ldate|log.Ltime|log.Lshortfile)
}

// Initialize should be called once at the beginning of the program to set up logging.
// defer Close() after calling this function. It sets the go log output to the file in
// the os temp directory. Returns an error if the log file cannot be opened.
func Initialize(daemon bool) error {
	f, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("could not open log file: %w", err)
	}

	// Set log format to include timestamp and file/line number
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	fmtS := "%s"
	if daemon {
		fmtS = "[DAEMON] %s"
	}
	InfoLog = log.New(f, fmt.Sprintf(fmtS, "INFO:"), log.Ldate|log.Ltime|log.Lshortfile)
	WarningLog = log.New(f, fmt.Sprintf(fmtS, "WARNING:"), log.Ldate|log.Ltime|log.Lshortfile)
	ErrorLog = log.New(f, fmt.Sprintf(fmtS, "ERROR:"), log.Ldate|log.Ltime|log.Lshortfile)

	globalLogFile = f
	return nil
}

func Close() {
	if globalLogFile == nil {
		return
	}
	_ = globalLogFile.Close()
	// Keep the log path visible so interactive runs always surface where errors landed.
	fmt.Println("wrote logs to " + logFileName)
}

// Every is used to log at most once every timeout duration.
type Every struct {
	timeout time.Duration
	timer   *time.Timer
}

func NewEvery(timeout time.Duration) *Every {
	return &Every{timeout: timeout}
}

// ShouldLog returns true if the timeout has passed since the last log.
func (e *Every) ShouldLog() bool {
	if e.timer == nil {
		e.timer = time.NewTimer(e.timeout)
		return true
	}

	select {
	case <-e.timer.C:
		e.timer.Reset(e.timeout)
		return true
	default:
		return false
	}
}

package tracking

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"clinic-api/internal/config"

	"github.com/labstack/echo/v4"
)

type Level string

const (
	LevelDebug   Level = "debug"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

const (
	logMaxSize = 10 * 1024 * 1024
	logBackups = 5
)

var (
	logMu   sync.Mutex
	logFile *rollingWriter
)

// Init configures the process-wide standard logger. All existing log package
// calls are mirrored to stderr and the rotating application log.
func Init(release, environment string) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFile != nil {
		log.SetOutput(os.Stderr)
		_ = logFile.Close()
		logFile = nil
	}

	dir := filepath.Join(config.DataDir(), "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("[tracking] create log directory: %v", err)
		return
	}
	w, err := newRollingWriter(filepath.Join(dir, "clinic.log"), logMaxSize, logBackups)
	if err != nil {
		log.Printf("[tracking] open log file: %v", err)
		return
	}
	logFile = w
	log.SetOutput(io.MultiWriter(os.Stderr, w))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC)
	log.Printf("[tracking] local logging enabled path=%s environment=%s release=%s", w.path, environment, release)
}

// Flush closes the local log. Local writes are unbuffered; timeout is retained
// so existing shutdown callers need no special handling.
func Flush(timeout time.Duration) {
	_ = timeout
	logMu.Lock()
	defer logMu.Unlock()
	if logFile == nil {
		return
	}
	log.SetOutput(os.Stderr)
	if err := logFile.Close(); err != nil {
		log.Printf("[tracking] close log file: %v", err)
	}
	logFile = nil
}

func CaptureError(c echo.Context, err error) {
	if err == nil {
		return
	}
	if c != nil {
		log.Printf("[error] %s %s: %v", c.Request().Method, c.Request().URL.Path, err)
		return
	}
	log.Printf("[error] %v", err)
}

func CaptureMessage(c echo.Context, level Level, msg string) {
	CaptureMessageWith(c, level, msg, nil)
}

func CaptureMessageWith(c echo.Context, level Level, msg string, details map[string]any) {
	prefix := ""
	if c != nil {
		prefix = fmt.Sprintf("%s %s: ", c.Request().Method, c.Request().URL.Path)
	}
	if len(details) > 0 {
		log.Printf("[%s] %s%s details=%v", level, prefix, msg, details)
		return
	}
	log.Printf("[%s] %s%s", level, prefix, msg)
}

func Debug(c echo.Context, msg string) { CaptureMessage(c, LevelDebug, msg) }
func Warn(c echo.Context, msg string)  { CaptureMessage(c, LevelWarning, msg) }
func Info(c echo.Context, msg string)  { CaptureMessage(c, LevelInfo, msg) }

func WarnWith(c echo.Context, msg string, details map[string]any) {
	CaptureMessageWith(c, LevelWarning, msg, details)
}

func Fatal(msg string, err error) {
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
	log.Fatal(msg)
}

func Recover() {
	if r := recover(); r != nil {
		log.Printf("[panic] %v\n%s", r, debug.Stack())
		panic(r)
	}
}

func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[panic] %s %s: %v\n%s", c.Request().Method, c.Request().URL.Path, r, debug.Stack())
					panic(r)
				}
			}()

			err = next(c)
			if status := c.Response().Status; status >= 500 {
				msg := fmt.Sprintf("HTTP %d %s %s", status, c.Request().Method, c.Request().URL.Path)
				if err != nil {
					log.Printf("[error] %s: %v", msg, err)
				} else {
					log.Printf("[error] %s", msg)
				}
			}
			return err
		}
	}
}

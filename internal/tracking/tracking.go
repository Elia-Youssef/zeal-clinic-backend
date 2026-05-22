package tracking

import (
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
)

var enabled bool

func Init(dsn, release, environment string) {
	if dsn == "" {
		log.Println("[tracking] Sentry disabled (no DSN)")
		return
	}
	if err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Release:          release,
		Environment:      environment,
		AttachStacktrace: true,
	}); err != nil {
		log.Printf("[tracking] init failed: %v", err)
		return
	}
	enabled = true
	log.Printf("[tracking] Sentry enabled (env=%s release=%s)", environment, release)
}

func Flush(timeout time.Duration) {
	if enabled {
		sentry.Flush(timeout)
	}
}

func Hub(c echo.Context) *sentry.Hub {
	if c != nil {
		if h := sentry.GetHubFromContext(c.Request().Context()); h != nil {
			return h
		}
	}
	return sentry.CurrentHub()
}

func CaptureError(c echo.Context, err error) {
	if !enabled || err == nil {
		return
	}
	Hub(c).CaptureException(err)
}

func CaptureMessage(c echo.Context, level sentry.Level, msg string) {
	CaptureMessageWith(c, level, msg, nil)
}

// CaptureMessageWith captures a message with a structured context block attached
// so the details (not just the summary) land on the Sentry event.
func CaptureMessageWith(c echo.Context, level sentry.Level, msg string, details map[string]any) {
	if !enabled {
		return
	}
	h := Hub(c)
	h.WithScope(func(s *sentry.Scope) {
		s.SetLevel(level)
		if len(details) > 0 {
			s.SetContext("details", details)
		}
		h.CaptureMessage(msg)
	})
}

func Warn(c echo.Context, msg string) { CaptureMessage(c, sentry.LevelWarning, msg) }
func Info(c echo.Context, msg string) { CaptureMessage(c, sentry.LevelInfo, msg) }

func WarnWith(c echo.Context, msg string, details map[string]any) {
	CaptureMessageWith(c, sentry.LevelWarning, msg, details)
}

func Fatal(msg string, err error) {
	if enabled {
		if err != nil {
			sentry.CaptureException(fmt.Errorf("%s: %w", msg, err))
		} else {
			CaptureMessage(nil, sentry.LevelFatal, msg)
		}
		sentry.Flush(5 * time.Second)
	}
	if err != nil {
		log.Fatalf("%s: %v", msg, err)
	}
	log.Fatal(msg)
}

func Recover() {
	if r := recover(); r != nil {
		if enabled {
			sentry.CurrentHub().Recover(r)
			sentry.Flush(2 * time.Second)
		}
		panic(r)
	}
}

func Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			if !enabled {
				return next(c)
			}
			hub := sentry.CurrentHub().Clone()
			hub.Scope().SetRequest(c.Request())
			ctx := sentry.SetHubOnContext(c.Request().Context(), hub)
			c.SetRequest(c.Request().WithContext(ctx))

			defer func() {
				if r := recover(); r != nil {
					hub.RecoverWithContext(ctx, r)
					panic(r)
				}
			}()

			err = next(c)
			if status := c.Response().Status; status >= 500 {
				if err != nil {
					hub.CaptureException(err)
				} else {
					hub.CaptureMessage(fmt.Sprintf("HTTP %d %s %s", status, c.Request().Method, c.Request().URL.Path))
				}
			}
			return err
		}
	}
}

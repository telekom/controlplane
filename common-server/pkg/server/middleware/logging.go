package middleware

import (
	"io"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/google/uuid"
)

type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
)

type LoggerOpts struct {
	Output io.Writer
	Format LogFormat
}

type LoggerOption func(*LoggerOpts)

func WithOutput(w io.Writer) LoggerOption {
	return func(o *LoggerOpts) {
		o.Output = w
	}
}

const jsonFormat = `{"time":"${time}","ip":"${ip}","host":"${host}","method":"${method}","path":"${path}","status":${status},"latency":"${latency}","queryParams":"${queryParams}", "cid": "${cid}"}` + "\n"

var formats = map[LogFormat]string{
	LogFormatJSON: jsonFormat,
}

var logCorrelationId = func(output logger.Buffer, c *fiber.Ctx, _ *logger.Data, _ string) (int, error) {
	cid := c.Locals("cid")
	if cid == nil {
		return 0, nil
	}
	return output.WriteString(cid.(string))
}

func NewLogger(opts ...LoggerOption) fiber.Handler {
	o := &LoggerOpts{
		Output: os.Stderr,
		Format: LogFormatJSON,
	}
	for _, opt := range opts {
		opt(o)
	}

	return logger.New(logger.Config{
		Output: o.Output,
		CustomTags: map[string]logger.LogFunc{
			"cid": logCorrelationId,
		},
		Format:       formats[o.Format],
		TimeFormat:   time.RFC3339,
		TimeZone:     "UTC",
		TimeInterval: 500 * time.Millisecond,
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/healthz" || c.Path() == "/readyz"
		},
	})
}

func NewContextLogger(log *logr.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx := c.UserContext()
		cid := uuid.NewString()
		ctx = logr.NewContext(ctx, log.WithValues("cid", cid)) // correlation id
		c.SetUserContext(ctx)
		c.Locals("cid", cid)
		return c.Next()
	}
}

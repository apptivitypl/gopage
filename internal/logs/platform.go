//go:build !js

package logs

import (
	"log/slog"
	"os"
)

func platform() string {
	if os.Getenv(ServiceVar) != "" {
		return FormatGCP
	}
	return ""
}

func console(opts Options) slog.Handler {
	return slog.NewJSONHandler(opts.Writer, &slog.HandlerOptions{Level: opts.Level})
}

package debuglog

import (
	"io"
	"log/slog"
	"os"
)

// Init configures the global slog logger to write to both stdout and a log file.
// Call once at startup. All packages can then use slog.Debug / Info / Warn / Error.
func Init(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	var w io.Writer
	if err != nil {
		w = os.Stdout
	} else {
		w = io.MultiWriter(os.Stdout, f)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
	if err != nil {
		slog.Warn("could not open log file, stdout only", "err", err)
	} else {
		slog.Info("logging initialised", "file", path)
	}
}

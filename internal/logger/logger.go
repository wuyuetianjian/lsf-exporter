package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = -4
	LevelInfo  Level = 0
	LevelWarn  Level = 4
	LevelError Level = 8
)

type Logger struct {
	out   io.Writer
	level Level
	mu    sync.Mutex
}

func New(out io.Writer, level Level) *Logger {
	return &Logger{out: out, level: level}
}

func (l *Logger) Debug(message string, attrs ...any) {
	l.write(LevelDebug, "debug", message, attrs...)
}

func (l *Logger) Info(message string, attrs ...any) {
	l.write(LevelInfo, "info", message, attrs...)
}

func (l *Logger) Warn(message string, attrs ...any) {
	l.write(LevelWarn, "warn", message, attrs...)
}

func (l *Logger) Error(message string, attrs ...any) {
	l.write(LevelError, "error", message, attrs...)
}

func (l *Logger) write(level Level, levelName string, message string, attrs ...any) {
	if level < l.level {
		return
	}
	entry := map[string]any{
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
		"level":   levelName,
		"message": message,
	}
	for i := 0; i+1 < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok || key == "" {
			continue
		}
		entry[key] = fmt.Sprint(attrs[i+1])
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_ = json.NewEncoder(l.out).Encode(entry)
}

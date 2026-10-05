// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"context"
	"log/slog"
	"slices"

	"go.uber.org/zap/zapcore"
)

// ZapCore returns a zap core that writes to l, so that a library that logs
// with zap (the certificate library) goes through the same level, format and
// redaction as the rest of the log. The zap logger name becomes the "logger"
// attribute and every record carries source=<source>.
func ZapCore(l *slog.Logger, source string) zapcore.Core {
	return &zapBridge{l: l.With("source", source)}
}

type zapBridge struct {
	l      *slog.Logger
	fields []zapcore.Field
}

func (b *zapBridge) Enabled(lvl zapcore.Level) bool {
	return b.l.Enabled(context.Background(), slogLevel(lvl))
}

func (b *zapBridge) With(fields []zapcore.Field) zapcore.Core {
	return &zapBridge{l: b.l, fields: append(slices.Clone(b.fields), fields...)}
}

func (b *zapBridge) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if b.Enabled(ent.Level) {
		return ce.AddCore(ent, b)
	}
	return ce
}

func (b *zapBridge) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range b.fields {
		f.AddTo(enc)
	}
	for _, f := range fields {
		f.AddTo(enc)
	}
	rec := slog.NewRecord(ent.Time, slogLevel(ent.Level), ent.Message, 0)
	if ent.LoggerName != "" {
		rec.AddAttrs(slog.String("logger", ent.LoggerName))
	}
	keys := make([]string, 0, len(enc.Fields))
	for k := range enc.Fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		name := k
		if k == "identifier" {
			name = "domain" // the library's name for a domain; "identifier" is redacted as a login name
		}
		rec.AddAttrs(slog.Any(name, enc.Fields[k]))
	}
	return b.l.Handler().Handle(context.Background(), rec)
}

func (b *zapBridge) Sync() error { return nil }

func slogLevel(l zapcore.Level) slog.Level {
	switch {
	case l <= zapcore.DebugLevel:
		return slog.LevelDebug
	case l == zapcore.InfoLevel:
		return slog.LevelInfo
	case l == zapcore.WarnLevel:
		return slog.LevelWarn
	}
	return slog.LevelError
}

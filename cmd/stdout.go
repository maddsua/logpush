package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/maddsua/logpush"
)

type StdoutWriter struct {
}

func (this *StdoutWriter) Type() string {
	return "stdout"
}

func (this *StdoutWriter) WriteEntry(ctx context.Context, entry logpush.LogEntry) error {
	slog.Info(fmt.Sprintf("STDOUT %v %s %s", entry.Timestamp, entry.LogLevel.String(), entry.Message))
	return nil
}

func (this *StdoutWriter) WriteBatch(ctx context.Context, batch []logpush.LogEntry) error {
	for _, entry := range batch {
		this.WriteEntry(ctx, entry)
	}
	return nil
}

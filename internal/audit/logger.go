package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/aborroy/alfresco-cli/internal/validation"
)

type Logger struct {
	mu      sync.RWMutex
	path    string
	events  []validation.AuditEvent
	byTrace map[string][]validation.AuditEvent
}

func NewLogger(path string) *Logger {
	return &Logger{
		path:    path,
		byTrace: make(map[string][]validation.AuditEvent),
	}
}

func (l *Logger) Append(evt validation.AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now().UTC()
	}
	l.events = append(l.events, evt)
	l.byTrace[evt.TraceID] = append(l.byTrace[evt.TraceID], evt)

	if l.path == "" {
		return nil
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(evt)
}

func (l *Logger) ByTrace(traceID string) []validation.AuditEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()
	src := l.byTrace[traceID]
	out := make([]validation.AuditEvent, len(src))
	copy(out, src)
	return out
}

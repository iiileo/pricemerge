package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxLogLines   = 500
	detailSep     = "\x1f"
	detailItemSep = "\x1e"
)

// LogEntry is one log line shown in the UI.
type LogEntry struct {
	Time    string   `json:"time"`
	Level   string   `json:"level"` // info | ok | err
	Message string   `json:"message"`
	Details []string `json:"details"`
}

// LogInfo describes log file state.
type LogInfo struct {
	Path  string     `json:"path"`
	Count int        `json:"count"`
	Lines []LogEntry `json:"lines"`
}

type appLog struct {
	mu    sync.Mutex
	path  string
	lines []LogEntry
}

func newAppLog() *appLog {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "pricemerge")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "pricemerge.log")
	l := &appLog{path: path}
	l.load()
	return l
}

func (l *appLog) load() {
	data, err := os.ReadFile(l.path)
	if err != nil {
		l.lines = nil
		return
	}
	raw := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	lines := make([]LogEntry, 0, len(raw))
	for _, row := range raw {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		lines = append(lines, parseLogLine(row))
	}
	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	l.lines = lines
}

func parseLogLine(row string) LogEntry {
	// format: 2006-01-02 15:04:05 [info] message[\x1fdetail\x1edetail...]
	if len(row) >= 28 && row[19] == ' ' && row[20] == '[' {
		end := strings.IndexByte(row[21:], ']')
		if end >= 0 {
			level := row[21 : 21+end]
			rest := strings.TrimSpace(row[22+end:])
			msg, details := splitDetails(rest)
			return LogEntry{Time: row[:19], Level: level, Message: msg, Details: details}
		}
	}
	msg, details := splitDetails(row)
	return LogEntry{Time: "", Level: "info", Message: msg, Details: details}
}

func splitDetails(rest string) (string, []string) {
	parts := strings.SplitN(rest, detailSep, 2)
	msg := strings.TrimSpace(parts[0])
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return msg, nil
	}
	raw := strings.Split(parts[1], detailItemSep)
	out := make([]string, 0, len(raw))
	for _, d := range raw {
		d = strings.TrimSpace(d)
		if d != "" {
			out = append(out, d)
		}
	}
	if len(out) == 0 {
		return msg, nil
	}
	return msg, out
}

func (l *appLog) append(level, message string, details []string) LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	level = strings.TrimSpace(level)
	if level == "" {
		level = "info"
	}
	message = strings.TrimSpace(message)
	clean := make([]string, 0, len(details))
	for _, d := range details {
		d = strings.TrimSpace(d)
		if d != "" {
			clean = append(clean, d)
		}
	}
	entry := LogEntry{
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Level:   level,
		Message: message,
		Details: clean,
	}
	l.lines = append(l.lines, entry)
	if len(l.lines) > maxLogLines {
		l.lines = l.lines[len(l.lines)-maxLogLines:]
	}
	line := fmt.Sprintf("%s [%s] %s", entry.Time, entry.Level, entry.Message)
	if len(clean) > 0 {
		line += detailSep + strings.Join(clean, detailItemSep)
	}
	line += "\n"
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		_, _ = f.WriteString(line)
		_ = f.Close()
	}
	return entry
}

func (l *appLog) clear() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
	return os.WriteFile(l.path, nil, 0o644)
}

func (l *appLog) exportTo(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("请选择保存路径")
	}
	var b strings.Builder
	b.WriteString("# PriceMerge 操作日志\n")
	b.WriteString("# 导出时间: ")
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString("\n\n")
	for _, entry := range l.lines {
		b.WriteString(entry.Time)
		b.WriteString(" [")
		b.WriteString(entry.Level)
		b.WriteString("] ")
		b.WriteString(entry.Message)
		b.WriteByte('\n')
		for _, d := range entry.Details {
			b.WriteString("  - ")
			b.WriteString(d)
			b.WriteByte('\n')
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func (l *appLog) info() LogInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	return LogInfo{Path: l.path, Count: len(l.lines), Lines: reverseCopy(l.lines)}
}

func reverseCopy(in []LogEntry) []LogEntry {
	out := make([]LogEntry, len(in))
	for i := range in {
		out[i] = in[len(in)-1-i]
		if len(out[i].Details) > 0 {
			out[i].Details = append([]string(nil), out[i].Details...)
		}
	}
	return out
}

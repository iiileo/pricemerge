package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxLogLines = 500

// LogEntry is one log line shown in the UI.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"` // info | ok | err
	Message string `json:"message"`
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
	// format: 2006-01-02 15:04:05 [info] message
	if len(row) >= 28 && row[19] == ' ' && row[20] == '[' {
		end := strings.IndexByte(row[21:], ']')
		if end >= 0 {
			level := row[21 : 21+end]
			msg := strings.TrimSpace(row[22+end:])
			return LogEntry{Time: row[:19], Level: level, Message: msg}
		}
	}
	return LogEntry{Time: "", Level: "info", Message: row}
}

func (l *appLog) append(level, message string) LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	level = strings.TrimSpace(level)
	if level == "" {
		level = "info"
	}
	message = strings.TrimSpace(message)
	entry := LogEntry{
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Level:   level,
		Message: message,
	}
	l.lines = append(l.lines, entry)
	if len(l.lines) > maxLogLines {
		l.lines = l.lines[len(l.lines)-maxLogLines:]
	}
	line := fmt.Sprintf("%s [%s] %s\n", entry.Time, entry.Level, entry.Message)
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

func (l *appLog) info() LogInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LogEntry, len(l.lines))
	copy(out, l.lines)
	// newest first for UI
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return LogInfo{Path: l.path, Count: len(l.lines), Lines: out}
}

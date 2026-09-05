package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppLogPersistAndClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.log")
	l := &appLog{path: path}
	l.append("ok", "hello")
	l.append("err", "boom")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected log file content")
	}

	info := l.info()
	if info.Count != 2 || len(info.Lines) != 2 {
		t.Fatalf("%#v", info)
	}
	if err := l.clear(); err != nil {
		t.Fatal(err)
	}
	info = l.info()
	if info.Count != 0 {
		t.Fatalf("cleared count=%d", info.Count)
	}
	data, _ = os.ReadFile(path)
	if len(data) != 0 {
		t.Fatalf("file not empty: %q", data)
	}
}

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type appPrefs struct {
	BasePath    string `json:"basePath"`
	MappingPath string `json:"mappingPath"`
}

func prefsPath() string {
	if prefsFileOverride != "" {
		return prefsFileOverride
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "pricemerge")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, "prefs.json")
}

// prefsFileOverride lets tests avoid touching the real user config.
var prefsFileOverride string

func loadPrefs() appPrefs {
	data, err := os.ReadFile(prefsPath())
	if err != nil {
		return appPrefs{}
	}
	var p appPrefs
	if err := json.Unmarshal(data, &p); err != nil {
		return appPrefs{}
	}
	return p
}

func savePrefs(p appPrefs) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(prefsPath(), data, 0o644)
}

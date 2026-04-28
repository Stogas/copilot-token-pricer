package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Workspace struct {
	ID           string
	Path         string
	SessionFiles []string
}

func defaultStorageRoot() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Code", "User", "workspaceStorage")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Code", "User", "workspaceStorage")
	default:
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		return filepath.Join(config, "Code", "User", "workspaceStorage")
	}
}

func discoverWorkspaces(storageRoot string) ([]Workspace, error) {
	if storageRoot == "" {
		storageRoot = defaultStorageRoot()
	}

	entries, err := os.ReadDir(storageRoot)
	if err != nil {
		return nil, err
	}

	var workspaces []Workspace
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		workspaceDir := filepath.Join(storageRoot, entry.Name())
		sessionsDir := filepath.Join(workspaceDir, "chatSessions")
		sessionEntries, err := os.ReadDir(sessionsDir)
		if err != nil {
			continue
		}

		var files []string
		for _, sessionEntry := range sessionEntries {
			if sessionEntry.IsDir() || strings.ToLower(filepath.Ext(sessionEntry.Name())) != ".jsonl" {
				continue
			}
			files = append(files, filepath.Join(sessionsDir, sessionEntry.Name()))
		}
		if len(files) == 0 {
			continue
		}
		sort.Strings(files)

		workspaces = append(workspaces, Workspace{
			ID:           entry.Name(),
			Path:         resolveWorkspacePath(workspaceDir),
			SessionFiles: files,
		})
	}

	sort.Slice(workspaces, func(i, j int) bool {
		left := workspaces[i].Path
		if left == "" {
			left = workspaces[i].ID
		}
		right := workspaces[j].Path
		if right == "" {
			right = workspaces[j].ID
		}
		return strings.ToLower(left) < strings.ToLower(right)
	})

	return workspaces, nil
}

func resolveWorkspacePath(workspaceDir string) string {
	content, err := os.ReadFile(filepath.Join(workspaceDir, "workspace.json"))
	if err != nil {
		return ""
	}

	var data struct {
		Folder    string `json:"folder"`
		Workspace string `json:"workspace"`
	}
	if err := json.Unmarshal(content, &data); err != nil {
		return ""
	}

	raw := data.Folder
	if raw == "" {
		raw = data.Workspace
	}
	return decodeWorkspaceURI(raw)
}

func decodeWorkspaceURI(raw string) string {
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme == "file" {
		decoded, _ := url.PathUnescape(parsed.Path)
		if runtime.GOOS == "windows" && len(decoded) >= 3 && decoded[0] == '/' && decoded[2] == ':' {
			decoded = decoded[1:]
		}
		if parsed.Host != "" && runtime.GOOS == "windows" {
			return `\\` + parsed.Host + filepath.FromSlash(decoded)
		}
		return filepath.FromSlash(decoded)
	}

	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw
	}
	return decoded
}

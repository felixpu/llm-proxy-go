// Package paths provides path management for different runtime environments.
// Supports development mode, binary mode, and installed mode.
package paths

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	basePath string
	dataPath string
	once     sync.Once
)

// IsBinaryMode returns true if running as a compiled binary (not go run).
func IsBinaryMode() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// go run creates temp binaries in /tmp or similar
	return !isInTempDir(exe)
}

func isInTempDir(path string) bool {
	tempDir := os.TempDir()
	return len(path) > len(tempDir) && path[:len(tempDir)] == tempDir
}

// GetBasePath returns the base path for the application.
// In dev mode: the go/ directory
// In binary mode: the directory containing the executable
func GetBasePath() string {
	once.Do(initPaths)
	return basePath
}

// GetDataPath returns the data directory path.
// Creates the directory if it doesn't exist.
func GetDataPath() string {
	once.Do(initPaths)
	return dataPath
}

// GetDBPath returns the full path to the SQLite database file.
func GetDBPath() string {
	// Allow override via environment variable
	if dbPath := os.Getenv("LLM_PROXY_DB"); dbPath != "" {
		return dbPath
	}
	return filepath.Join(GetDataPath(), "llm-proxy.db")
}

// GetStaticDir returns the path to static files directory.
func GetStaticDir() string {
	return filepath.Join(GetBasePath(), "static")
}

// GetTemplatesDir returns the path to templates directory.
func GetTemplatesDir() string {
	return filepath.Join(GetBasePath(), "templates")
}

func initPaths() {
	if IsBinaryMode() {
		exe, _ := os.Executable()
		basePath = filepath.Dir(exe)
	} else {
		// Development mode: find the go/ directory
		wd, _ := os.Getwd()
		basePath = findGoDir(wd)
	}

	// Data path: check env var first, then default to data/ under base
	if dp := os.Getenv("LLM_PROXY_DATA_DIR"); dp != "" {
		dataPath = dp
	} else {
		// In dev mode, use the parent project's data directory
		if !IsBinaryMode() {
			dataPath = filepath.Join(filepath.Dir(basePath), "data")
		} else {
			dataPath = filepath.Join(basePath, "data")
		}
	}

	// Ensure data directory exists
	_ = os.MkdirAll(dataPath, 0755)
}

// findGoDir walks up the directory tree to find the go/ directory.
func findGoDir(start string) string {
	dir := start
	for {
		if filepath.Base(dir) == "go" {
			return dir
		}
		// Check if go/ exists as a subdirectory
		goDir := filepath.Join(dir, "go")
		if info, err := os.Stat(goDir); err == nil && info.IsDir() {
			return goDir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root, return current working directory
			return start
		}
		dir = parent
	}
}

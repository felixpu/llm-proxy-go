package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/llm-proxy-go/internal/pkg/paths"
)

func TestDatabasePathDefault(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("LLM_PROXY_DB")

	cfg := DefaultConfig()
	cfg.Database.Path = paths.GetDBPath()

	// Should use default path: data/llm-proxy.db
	if !filepath.IsAbs(cfg.Database.Path) {
		// In test, it might be relative or absolute depending on mode
		t.Logf("Database path: %s", cfg.Database.Path)
	}

	// Path should end with llm-proxy.db
	if filepath.Base(cfg.Database.Path) != "llm-proxy.db" {
		t.Errorf("Expected database filename to be llm-proxy.db, got %s", filepath.Base(cfg.Database.Path))
	}

	// Path should be in data directory
	if filepath.Base(filepath.Dir(cfg.Database.Path)) != "data" {
		t.Errorf("Expected database to be in data directory, got %s", filepath.Dir(cfg.Database.Path))
	}
}

func TestDatabasePathEnvOverride(t *testing.T) {
	// Set environment variable
	customPath := "/custom/path/test.db"
	os.Setenv("LLM_PROXY_DB", customPath)
	defer os.Unsetenv("LLM_PROXY_DB")

	cfg := DefaultConfig()
	cfg.Database.Path = paths.GetDBPath()

	// Apply environment overrides
	applyEnvOverrides(cfg)

	// Should use custom path from environment variable
	if cfg.Database.Path != customPath {
		t.Errorf("Expected database path to be %s, got %s", customPath, cfg.Database.Path)
	}
}

func TestLoadConfigDatabasePath(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("LLM_PROXY_DB")

	// Note: Load() will fail if database doesn't exist, but we can test the path logic
	cfg := DefaultConfig()
	cfg.Database.Path = paths.GetDBPath()

	// Verify default path structure
	if filepath.Base(cfg.Database.Path) != "llm-proxy.db" {
		t.Errorf("Expected default database filename to be llm-proxy.db, got %s", filepath.Base(cfg.Database.Path))
	}
}

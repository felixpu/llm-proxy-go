package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/config"
)

func testRotationConfig() config.LogRotationConfig {
	return config.LogRotationConfig{
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 1,
		Compress:   false,
	}
}

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := newLogger("INFO", tmpDir, testRotationConfig())
	require.NoError(t, err)
	require.NotNil(t, logger)

	logger.Info("test message")
	_ = logger.Sync()

	// Verify log file was created.
	logFile := filepath.Join(tmpDir, "llm-proxy.log")
	_, err = os.Stat(logFile)
	require.NoError(t, err)
}

func TestNewLoggerLevels(t *testing.T) {
	tmpDir := t.TempDir()
	rotation := testRotationConfig()

	levels := []string{"DEBUG", "INFO", "WARN", "ERROR", "invalid"}
	for _, level := range levels {
		logger, err := newLogger(level, tmpDir, rotation)
		require.NoError(t, err)
		require.NotNil(t, logger)
	}
}

func TestNewLoggerCreatesDir(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "nested", "logs")

	logger, err := newLogger("INFO", tmpDir, testRotationConfig())
	require.NoError(t, err)
	require.NotNil(t, logger)

	// Verify nested directory was created.
	info, err := os.Stat(tmpDir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestGetLogDir(t *testing.T) {
	// Default value.
	t.Setenv("LLM_PROXY_LOGS_DIR", "")
	require.Equal(t, "logs", getLogDir())

	// Custom value.
	t.Setenv("LLM_PROXY_LOGS_DIR", "/tmp/custom-logs")
	require.Equal(t, "/tmp/custom-logs", getLogDir())
}

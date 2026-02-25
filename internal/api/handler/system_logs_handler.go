package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const systemLogFile = "logs/llm-proxy.log"

// parsedLogEntry represents the frontend-expected log format.
type parsedLogEntry struct {
	Level     string `json:"level"`
	WorkerID  string `json:"worker_id"`
	Timestamp string `json:"timestamp"`
	Caller    string `json:"caller"`
	Message   string `json:"message"`
}

// fixedZapKeys are the well-known zap fields extracted separately.
var fixedZapKeys = map[string]bool{
	"level": true, "ts": true, "msg": true, "worker": true, "caller": true,
}

// parseZapLogLine parses a zap JSON log line into frontend-expected format.
// All extra structured fields (status, method, path, latency, ip, …) are
// appended to the message as "key=value" pairs.
func parseZapLogLine(line string) *parsedLogEntry {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return &parsedLogEntry{
			Level:     "INFO",
			Timestamp: time.Now().UTC().Format("2006-01-02 15:04:05"),
			Message:   line,
		}
	}

	// Extract fixed fields.
	level := strings.ToUpper(fmt.Sprintf("%v", raw["level"]))
	if level == "WARN" {
		level = "WARNING"
	}

	msg, _ := raw["msg"].(string)
	worker, _ := raw["worker"].(string)
	caller, _ := raw["caller"].(string)

	var timestamp string
	if tsVal, ok := raw["ts"].(float64); ok {
		t := time.Unix(int64(tsVal), int64((tsVal-float64(int64(tsVal)))*1e9)).UTC()
		timestamp = t.Format("2006-01-02 15:04:05")
	} else {
		timestamp = time.Now().UTC().Format("2006-01-02 15:04:05")
	}

	// Collect remaining fields as "key=value" pairs (sorted for stability).
	var extras []string
	for k, v := range raw {
		if fixedZapKeys[k] {
			continue
		}
		extras = append(extras, fmt.Sprintf("%s=%v", k, v))
	}
	// Sort for deterministic output.
	for i := 1; i < len(extras); i++ {
		for j := i; j > 0 && extras[j] < extras[j-1]; j-- {
			extras[j], extras[j-1] = extras[j-1], extras[j]
		}
	}

	message := msg
	if len(extras) > 0 {
		message = msg + " " + strings.Join(extras, " ")
	}

	return &parsedLogEntry{
		Level:     level,
		WorkerID:  worker,
		Timestamp: timestamp,
		Caller:    caller,
		Message:   message,
	}
}

// shouldIncludeLine checks if a line matches the level and search criteria.
func shouldIncludeLine(line, level, search string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}

	if level != "" {
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err == nil {
			entryLevel := strings.ToUpper(fmt.Sprintf("%v", raw["level"]))
			if entryLevel == "WARN" {
				entryLevel = "WARNING"
			}
			if !strings.EqualFold(entryLevel, level) {
				return false
			}
		} else {
			upperLine := strings.ToUpper(line)
			upperLevel := strings.ToUpper(level)
			if !strings.Contains(upperLine, upperLevel) {
				return false
			}
		}
	}

	if search != "" {
		if !strings.Contains(strings.ToLower(line), strings.ToLower(search)) {
			return false
		}
	}

	return true
}

// StreamSystemLogs streams system logs using Server-Sent Events (SSE).
// GET /api/system-logs/stream
func StreamSystemLogs(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	level := c.Query("level")
	search := c.Query("search")

	w := c.Writer

	file, err := os.Open(systemLogFile)
	if err != nil {
		data, _ := json.Marshal(gin.H{"message": "Failed to open log file"})
		fmt.Fprintf(w, "data: %s\n\n", data)
		w.Flush()
		return
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > 100 {
			lines = lines[1:]
		}
	}

	for _, line := range lines {
		if shouldIncludeLine(line, level, search) {
			parsed := parseZapLogLine(line)
			if parsed != nil {
				data, _ := json.Marshal(parsed)
				fmt.Fprintf(w, "data: %s\n\n", data)
			}
		}
	}
	w.Flush()

	c.Stream(func(wr io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		default:
		}

		file.Seek(0, io.SeekEnd)
		reader := bufio.NewReader(file)

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				break
			}

			line = strings.TrimSuffix(line, "\n")
			if shouldIncludeLine(line, level, search) {
				parsed := parseZapLogLine(line)
				if parsed != nil {
					data, _ := json.Marshal(parsed)
					fmt.Fprintf(w, "data: %s\n\n", data)
					w.Flush()
				}
			}
		}

		return true
	})
}

// GetSystemLogEntries returns historical log entries.
// GET /api/system-logs
func GetSystemLogEntries(c *gin.Context) {
	linesParam := c.DefaultQuery("lines", "100")
	var maxLines int
	_, err := fmt.Sscanf(linesParam, "%d", &maxLines)
	if err != nil || maxLines < 1 {
		maxLines = 100
	}
	if maxLines > 10000 {
		maxLines = 10000
	}

	level := c.Query("level")
	search := c.Query("search")

	if _, err := os.Stat(systemLogFile); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"lines": []any{},
			"total": 0,
			"file":  systemLogFile,
		})
		return
	}

	file, err := os.Open(systemLogFile)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to read log file")
		return
	}
	defer file.Close()

	var allEntries []*parsedLogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if shouldIncludeLine(line, level, search) {
			parsed := parseZapLogLine(line)
			if parsed != nil {
				allEntries = append(allEntries, parsed)
			}
		}
	}

	total := len(allEntries)
	start := total - maxLines
	if start < 0 {
		start = 0
	}
	resultEntries := allEntries[start:]

	c.JSON(http.StatusOK, gin.H{
		"lines": resultEntries,
		"total": total,
		"file":  systemLogFile,
	})
}

// ClearSystemLogEntries clears the system log file.
// POST /api/system-logs/clear
func ClearSystemLogEntries(c *gin.Context) {
	if _, err := os.Stat(systemLogFile); os.IsNotExist(err) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "System logs cleared",
		})
		return
	}

	err := os.Truncate(systemLogFile, 0)
	if err != nil {
		errorResponse(c, http.StatusInternalServerError, "Failed to clear log file")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "System logs cleared",
	})
}

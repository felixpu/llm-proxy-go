//go:build !integration && !e2e
// +build !integration,!e2e

package handler

import (
	"strings"
	"testing"
)

func TestParseZapLogLine(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantLevel      string
		wantCaller     string
		wantMsgContain string
	}{
		{
			name:           "完整结构化日志",
			input:          `{"level":"info","ts":1771506915.3,"caller":"middleware/middleware.go:22","msg":"request","status":200,"method":"GET","path":"/model-provider","latency":0.024,"ip":"::1"}`,
			wantLevel:      "INFO",
			wantCaller:     "middleware/middleware.go:22",
			wantMsgContain: "request",
		},
		{
			name:           "包含 status、method、path",
			input:          `{"level":"info","ts":1771506915.3,"caller":"middleware/middleware.go:22","msg":"request","status":200,"method":"GET","path":"/model-provider"}`,
			wantLevel:      "INFO",
			wantCaller:     "middleware/middleware.go:22",
			wantMsgContain: "status=200",
		},
		{
			name:           "警告级别",
			input:          `{"level":"warn","ts":1771506915.3,"msg":"slow query","duration":1.5}`,
			wantLevel:      "WARNING",
			wantCaller:     "",
			wantMsgContain: "duration=1.5",
		},
		{
			name:           "非 JSON 行",
			input:          "plain text log line",
			wantLevel:      "INFO",
			wantCaller:     "",
			wantMsgContain: "plain text log line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseZapLogLine(tt.input)

			if result.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", result.Level, tt.wantLevel)
			}

			if result.Caller != tt.wantCaller {
				t.Errorf("Caller = %v, want %v", result.Caller, tt.wantCaller)
			}

			if !strings.Contains(result.Message, tt.wantMsgContain) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMsgContain)
			}

			// 验证结构化字段被拼接到消息中
			if strings.Contains(tt.input, `"status":200`) {
				if !strings.Contains(result.Message, "status=200") {
					t.Errorf("Message should contain status=200, got: %v", result.Message)
				}
			}

			if strings.Contains(tt.input, `"method":"GET"`) {
				if !strings.Contains(result.Message, "method=GET") {
					t.Errorf("Message should contain method=GET, got: %v", result.Message)
				}
			}
		})
	}
}

func TestParseZapLogLine_ExtraFieldsInMessage(t *testing.T) {
	input := `{"level":"info","ts":1771506915.3,"msg":"request","status":200,"method":"GET","path":"/api/test"}`
	result := parseZapLogLine(input)

	// 验证所有额外字段都在消息中
	requiredFields := []string{"status=200", "method=GET", "path=/api/test"}
	for _, field := range requiredFields {
		if !strings.Contains(result.Message, field) {
			t.Errorf("Message missing field %q, got: %v", field, result.Message)
		}
	}

	// 验证消息以 "request" 开头
	if !strings.HasPrefix(result.Message, "request ") {
		t.Errorf("Message should start with 'request ', got: %v", result.Message)
	}
}

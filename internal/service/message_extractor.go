package service

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

// systemInjectionRe matches system-injected XML tags from Claude Code clients.
var systemInjectionRe = regexp.MustCompile(`(?s)<(?:system-reminder|command-name|command-message|command-args|local-command-caveat|local-command-stdout)>.*?</(?:system-reminder|command-name|command-message|command-args|local-command-caveat|local-command-stdout)>`)

// stripSystemInjections removes system-injected content from user messages
// so that routing decisions are based on actual user intent only.
func stripSystemInjections(text string) string {
	cleaned := systemInjectionRe.ReplaceAllString(text, "")
	return strings.TrimSpace(cleaned)
}

// MessageExtractor extracts analysis-relevant info from stored request_content JSON.
type MessageExtractor struct{}

// ExtractRoutingMessage extracts the last user message text from an AnthropicRequest,
// using the same logic as the routing classifier: find the last user message with
// text content, skip messages that are entirely tool_result, and strip system
// injection tags. This is the single source of truth for what text the routing
// rules evaluate against.
func ExtractRoutingMessage(req *models.AnthropicRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}

	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != "user" {
			continue
		}

		parts := msg.Content.GetParts()
		if len(parts) == 0 {
			continue
		}

		var textParts []string
		for _, part := range parts {
			if part.Type == "text" && part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}

		if len(textParts) > 0 {
			raw := strings.Join(textParts, "\n")
			return stripSystemInjections(raw)
		}
	}

	return ""
}

// ExtractFromLog extracts an ExtractedLogEntry from a RequestLog.
func (e *MessageExtractor) ExtractFromLog(log *models.RequestLog) *models.ExtractedLogEntry {
	entry := &models.ExtractedLogEntry{
		ID:              log.ID,
		TaskType:        log.TaskType,
		RoutingMethod:   log.RoutingMethod,
		MatchedRuleName: log.MatchedRuleName,
		IsInaccurate:    log.IsInaccurate,
	}

	if log.RequestContent == "" {
		entry.UserMessage = log.MessagePreview
		return entry
	}

	var req models.AnthropicRequest
	if err := json.Unmarshal([]byte(log.RequestContent), &req); err != nil {
		entry.UserMessage = log.MessagePreview
		return entry
	}

	userMsg := ExtractRoutingMessage(&req)
	if userMsg == "" {
		userMsg = log.MessagePreview
	}
	if len(userMsg) > 500 {
		userMsg = userMsg[:500] + "..."
	}
	entry.UserMessage = userMsg

	return entry
}

package service

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/user/llm-proxy-go/internal/models"
)

var systemReminderRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)

// trivialMessages are short messages with no routing signal.
var trivialMessages = map[string]bool{
	"continue": true, "yes": true, "ok": true, "no": true,
	"y": true, "n": true, "go": true, "next": true,
	"继续": true, "好的": true, "是": true, "否": true,
}

// MessageExtractor extracts analysis-relevant info from stored request_content JSON.
type MessageExtractor struct{}

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

	userMsg, summary := e.parseRequestContent(log.RequestContent)
	entry.UserMessage = userMsg
	entry.MessageSummary = summary

	if entry.UserMessage == "" {
		entry.UserMessage = log.MessagePreview
	}
	return entry
}

// parseRequestContent parses the stored JSON and returns (userMessage, summary).
func (e *MessageExtractor) parseRequestContent(content string) (string, string) {
	var req models.AnthropicRequest
	if err := json.Unmarshal([]byte(content), &req); err != nil {
		return "", ""
	}

	userMsg := e.extractLastPureUserMessage(&req)
	summary := e.buildMessageSummary(&req)
	return userMsg, summary
}

// extractLastPureUserMessage finds the last meaningful user text message,
// skipping tool_result messages, system-reminder tags, and trivial short messages.
func (e *MessageExtractor) extractLastPureUserMessage(req *models.AnthropicRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if msg.Role != "user" {
			continue
		}

		parts := msg.Content.GetParts()
		if len(parts) == 0 {
			continue
		}

		// Skip if all parts are tool_result
		allToolResult := true
		for _, p := range parts {
			if p.Type != "tool_result" {
				allToolResult = false
				break
			}
		}
		if allToolResult {
			continue
		}

		// Collect text parts, filtering system-reminder content
		var texts []string
		for _, p := range parts {
			if p.Type != "text" || p.Text == "" {
				continue
			}
			cleaned := systemReminderRe.ReplaceAllString(p.Text, "")
			cleaned = strings.TrimSpace(cleaned)
			if cleaned != "" {
				texts = append(texts, cleaned)
			}
		}

		combined := strings.Join(texts, "\n")
		combined = strings.TrimSpace(combined)

		// Skip trivial messages, keep looking backwards
		if trivialMessages[strings.ToLower(combined)] {
			continue
		}

		if len(combined) > 500 {
			combined = combined[:500] + "..."
		}
		return combined
	}
	return ""
}

// buildMessageSummary generates a compact structural summary of the conversation.
func (e *MessageExtractor) buildMessageSummary(req *models.AnthropicRequest) string {
	var userCount, asstCount, toolUseCount, toolResultCount int
	hasSys := req.System != nil && !req.System.IsEmpty()
	hasThinking := false

	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			userCount++
		case "assistant":
			asstCount++
		}
		for _, p := range msg.Content.GetParts() {
			switch p.Type {
			case "tool_use":
				toolUseCount++
			case "tool_result":
				toolResultCount++
			case "thinking":
				hasThinking = true
			}
		}
	}

	var b strings.Builder
	b.WriteString("msgs=")
	b.WriteString(strings.Join([]string{
		itoa(len(req.Messages)),
		"(user:", itoa(userCount),
		",asst:", itoa(asstCount), ")",
	}, ""))

	if toolUseCount > 0 {
		b.WriteString(" tools_used=")
		b.WriteString(itoa(toolUseCount))
	}
	if toolResultCount > 0 {
		b.WriteString(" tool_results=")
		b.WriteString(itoa(toolResultCount))
	}
	if hasSys {
		b.WriteString(" sys=true")
	}
	if hasThinking {
		b.WriteString(" thinking=true")
	}
	return b.String()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

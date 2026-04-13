package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/user/llm-proxy-go/internal/models"
)

type proxyOpenAIChatRequest struct {
	Model       string                `json:"model"`
	Messages    []proxyOpenAIMessage  `json:"messages"`
	MaxTokens   int                   `json:"max_tokens,omitempty"`
	Stream      bool                  `json:"stream,omitempty"`
	Temperature *float64              `json:"temperature,omitempty"`
	TopP        *float64              `json:"top_p,omitempty"`
	Stop        interface{}           `json:"stop,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
	Tools       []proxyOpenAIChatTool `json:"tools,omitempty"`
	ToolChoice  interface{}           `json:"tool_choice,omitempty"`
}

type proxyOpenAIMessage struct {
	Role       string                `json:"role"`
	Content    interface{}           `json:"content,omitempty"`
	ToolCalls  []proxyOpenAIChatCall `json:"tool_calls,omitempty"`
	ToolCallID string                `json:"tool_call_id,omitempty"`
}

type proxyOpenAIChatTool struct {
	Type     string                  `json:"type"`
	Function proxyOpenAIFunctionSpec `json:"function"`
}

type proxyOpenAIFunctionSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type proxyOpenAIChatCall struct {
	ID       string                  `json:"id,omitempty"`
	Type     string                  `json:"type"`
	Function proxyOpenAIFunctionCall `json:"function"`
}

type proxyOpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type proxyOpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type proxyOpenAIChatResponse struct {
	ID      string                      `json:"id"`
	Model   string                      `json:"model"`
	Choices []proxyOpenAIResponseChoice `json:"choices"`
	Usage   proxyOpenAIUsage            `json:"usage"`
}

type proxyOpenAIResponseChoice struct {
	Message      proxyOpenAIResponseMessage `json:"message"`
	FinishReason string                     `json:"finish_reason"`
}

type proxyOpenAIResponseMessage struct {
	Role      string                `json:"role"`
	Content   json.RawMessage       `json:"content"`
	ToolCalls []proxyOpenAIChatCall `json:"tool_calls"`
}

type proxyAnthropicResponsesEnvelope struct {
	ID         string          `json:"id"`
	Model      string          `json:"model"`
	Role       string          `json:"role"`
	StopReason string          `json:"stop_reason"`
	Usage      models.Usage    `json:"usage"`
	Output     json.RawMessage `json:"output"`
	Content    json.RawMessage `json:"content"`
	OutputText string          `json:"output_text"`
}

func resolveProxyAPIType(ctx context.Context, ep *models.Endpoint) (APIType, error) {
	if ep == nil || ep.Provider == nil {
		return "", fmt.Errorf("endpoint provider is nil")
	}

	apiType := APIType(strings.TrimSpace(ep.Provider.APIType))
	switch apiType {
	case "":
		return APITypeAnthropicMessages, nil
	case APITypeAuto:
		detected, err := DetectAPIType(ctx, ep.Provider.BaseURL, ep.Provider.APIKey)
		if err != nil {
			return "", fmt.Errorf("detect api type for provider %s: %w", ep.Provider.Name, err)
		}
		return detected, nil
	case APITypeAnthropicMessages, APITypeAnthropicResponses, APITypeOpenAIChat:
		return apiType, nil
	default:
		return "", fmt.Errorf("unsupported provider api_type %q", ep.Provider.APIType)
	}
}

func buildProxyUpstreamBody(req *models.AnthropicRequest, modelName string, apiType APIType) ([]byte, error) {
	switch apiType {
	case APITypeAnthropicMessages:
		proxyReq := *req
		proxyReq.Model = modelName
		return json.Marshal(&proxyReq)
	case APITypeAnthropicResponses:
		return buildAnthropicResponsesBody(req, modelName)
	case APITypeOpenAIChat:
		return buildOpenAIChatBody(req, modelName)
	default:
		return nil, fmt.Errorf("unsupported api type %q", apiType)
	}
}

func parseProxyUpstreamResponse(respBody []byte, apiType APIType) (*models.AnthropicResponse, error) {
	switch apiType {
	case APITypeAnthropicMessages:
		var resp models.AnthropicResponse
		if err := json.Unmarshal(respBody, &resp); err != nil {
			return nil, err
		}
		return &resp, nil
	case APITypeAnthropicResponses:
		return parseAnthropicResponsesResponse(respBody)
	case APITypeOpenAIChat:
		return parseOpenAIChatResponse(respBody)
	default:
		return nil, fmt.Errorf("unsupported api type %q", apiType)
	}
}

func buildAnthropicResponsesBody(req *models.AnthropicRequest, modelName string) ([]byte, error) {
	body := map[string]interface{}{
		"model":             modelName,
		"input":             req.Messages,
		"max_output_tokens": req.MaxTokens,
		"stream":            req.Stream,
	}

	if req.System != nil && !req.System.IsEmpty() {
		body["instructions"] = req.System
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		body["top_k"] = *req.TopK
	}
	if len(req.StopSequences) > 0 {
		body["stop_sequences"] = req.StopSequences
	}
	if len(req.Metadata) > 0 {
		body["metadata"] = req.Metadata
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.ToolChoice != nil {
		body["tool_choice"] = req.ToolChoice
	}
	if req.Thinking != nil {
		body["thinking"] = req.Thinking
	}

	return json.Marshal(body)
}

func buildOpenAIChatBody(req *models.AnthropicRequest, modelName string) ([]byte, error) {
	body := &proxyOpenAIChatRequest{
		Model:       modelName,
		Messages:    buildOpenAIChatMessages(req),
		MaxTokens:   req.MaxTokens,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Metadata:    req.Metadata,
	}

	if stop := buildOpenAIStop(req.StopSequences); stop != nil {
		body.Stop = stop
	}
	if len(req.Tools) > 0 {
		body.Tools = buildOpenAITools(req.Tools)
	}
	if req.ToolChoice != nil {
		body.ToolChoice = buildOpenAIToolChoice(req.ToolChoice)
	}

	return json.Marshal(body)
}

func buildOpenAIChatMessages(req *models.AnthropicRequest) []proxyOpenAIMessage {
	messages := make([]proxyOpenAIMessage, 0, len(req.Messages)+1)

	if content := buildOpenAIContentFromSystem(req.System); hasOpenAIContent(content) {
		messages = append(messages, proxyOpenAIMessage{
			Role:    "system",
			Content: content,
		})
	}

	for _, msg := range req.Messages {
		switch msg.Role {
		case "assistant":
			assistant := buildOpenAIAssistantMessage(msg)
			if hasOpenAIContent(assistant.Content) || len(assistant.ToolCalls) > 0 {
				messages = append(messages, assistant)
			}
		case "user":
			messages = append(messages, buildOpenAIUserMessages(msg)...)
		default:
			if content := buildOpenAIContentFromMessage(msg.Content); hasOpenAIContent(content) {
				messages = append(messages, proxyOpenAIMessage{
					Role:    msg.Role,
					Content: content,
				})
			}
		}
	}

	return messages
}

func buildOpenAIAssistantMessage(msg models.Message) proxyOpenAIMessage {
	if !msg.Content.IsArray {
		return proxyOpenAIMessage{
			Role:    "assistant",
			Content: msg.Content.Text,
		}
	}

	var textParts []models.ContentPart
	var toolCalls []proxyOpenAIChatCall

	for _, part := range msg.Content.Parts {
		switch part.Type {
		case "tool_use":
			toolCalls = append(toolCalls, proxyOpenAIChatCall{
				ID:   defaultToolCallID(part.ID),
				Type: "function",
				Function: proxyOpenAIFunctionCall{
					Name:      part.Name,
					Arguments: marshalJSONValue(part.Input),
				},
			})
		default:
			textParts = append(textParts, part)
		}
	}

	return proxyOpenAIMessage{
		Role:      "assistant",
		Content:   buildOpenAIContentFromParts(textParts, true),
		ToolCalls: toolCalls,
	}
}

func buildOpenAIUserMessages(msg models.Message) []proxyOpenAIMessage {
	if !msg.Content.IsArray {
		return []proxyOpenAIMessage{{
			Role:    "user",
			Content: msg.Content.Text,
		}}
	}

	var messages []proxyOpenAIMessage
	var userParts []models.ContentPart

	for _, part := range msg.Content.Parts {
		if part.Type == "tool_result" {
			messages = append(messages, proxyOpenAIMessage{
				Role:       "tool",
				ToolCallID: part.ToolUseID,
				Content:    stringifyToolResult(part.Content),
			})
			continue
		}
		userParts = append(userParts, part)
	}

	if content := buildOpenAIContentFromParts(userParts, true); hasOpenAIContent(content) {
		messages = append([]proxyOpenAIMessage{{
			Role:    "user",
			Content: content,
		}}, messages...)
	}

	return messages
}

func buildOpenAIContentFromMessage(content models.MessageContent) interface{} {
	if !content.IsArray {
		return content.Text
	}
	return buildOpenAIContentFromParts(content.Parts, true)
}

func buildOpenAIContentFromSystem(system *models.SystemPrompt) interface{} {
	if system == nil || system.IsEmpty() {
		return nil
	}
	if !system.IsArray {
		return system.Text
	}
	return buildOpenAIContentFromParts(system.Blocks, true)
}

func buildOpenAIContentFromParts(parts []models.ContentPart, preferArray bool) interface{} {
	items := make([]map[string]interface{}, 0, len(parts))

	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				items = append(items, map[string]interface{}{
					"type": "text",
					"text": part.Text,
				})
			}
		case "image":
			if url := imageSourceToDataURL(part.Source); url != "" {
				items = append(items, map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url": url,
					},
				})
			}
		case "thinking":
			if part.Thinking != "" {
				items = append(items, map[string]interface{}{
					"type": "text",
					"text": part.Thinking,
				})
			}
		}
	}

	if len(items) == 0 {
		return nil
	}
	if !preferArray && len(items) == 1 {
		if text, ok := items[0]["text"].(string); ok {
			return text
		}
	}
	return items
}

func buildOpenAITools(tools []models.Tool) []proxyOpenAIChatTool {
	out := make([]proxyOpenAIChatTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, proxyOpenAIChatTool{
			Type: "function",
			Function: proxyOpenAIFunctionSpec{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return out
}

func buildOpenAIToolChoice(toolChoice *models.ToolChoice) interface{} {
	if toolChoice == nil {
		return nil
	}

	switch toolChoice.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		return map[string]interface{}{
			"type": "function",
			"function": map[string]string{
				"name": toolChoice.Name,
			},
		}
	default:
		return toolChoice.Type
	}
}

func buildOpenAIStop(stopSequences []string) interface{} {
	switch len(stopSequences) {
	case 0:
		return nil
	case 1:
		return stopSequences[0]
	default:
		return stopSequences
	}
}

func parseOpenAIChatResponse(respBody []byte) (*models.AnthropicResponse, error) {
	var resp proxyOpenAIChatResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("openai chat response contained no choices")
	}

	choice := resp.Choices[0]
	content, err := parseOpenAIContentParts(choice.Message.Content)
	if err != nil {
		return nil, err
	}
	content = append(content, buildAnthropicToolUses(choice.Message.ToolCalls)...)

	role := choice.Message.Role
	if role == "" {
		role = "assistant"
	}

	return &models.AnthropicResponse{
		ID:         resp.ID,
		Type:       "message",
		Role:       role,
		Content:    content,
		Model:      resp.Model,
		StopReason: mapOpenAIFinishReason(choice.FinishReason),
		Usage: models.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

func parseAnthropicResponsesResponse(respBody []byte) (*models.AnthropicResponse, error) {
	var envelope proxyAnthropicResponsesEnvelope
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return nil, err
	}

	content, role, err := parseAnthropicResponsesOutput(envelope.Output)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		if content, err = parseAnthropicResponsesContent(envelope.Content); err != nil {
			return nil, err
		}
	}
	if len(content) == 0 && envelope.OutputText != "" {
		content = []models.ContentPart{{Type: "text", Text: envelope.OutputText}}
	}

	if role == "" {
		role = envelope.Role
	}
	if role == "" {
		role = "assistant"
	}

	return &models.AnthropicResponse{
		ID:         envelope.ID,
		Type:       "message",
		Role:       role,
		Content:    content,
		Model:      envelope.Model,
		StopReason: envelope.StopReason,
		Usage:      envelope.Usage,
	}, nil
}

func parseOpenAIContentParts(raw json.RawMessage) ([]models.ContentPart, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if text == "" {
			return nil, nil
		}
		return []models.ContentPart{{Type: "text", Text: text}}, nil
	}

	var parts []map[string]interface{}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("unsupported openai content format")
	}

	out := make([]models.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch stringValue(part["type"]) {
		case "text", "output_text":
			if text := textValue(part["text"]); text != "" {
				out = append(out, models.ContentPart{Type: "text", Text: text})
			}
		}
	}

	return out, nil
}

func parseAnthropicResponsesOutput(raw json.RawMessage) ([]models.ContentPart, string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, "", nil
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, "", fmt.Errorf("unsupported responses output format")
	}

	var role string
	var content []models.ContentPart
	for _, item := range items {
		if role == "" {
			role = stringValue(item["role"])
		}
		if rawContent, ok := item["content"]; ok {
			parts, err := parseAnthropicResponsesContentValue(rawContent)
			if err != nil {
				return nil, "", err
			}
			content = append(content, parts...)
			continue
		}
		if part, ok := parseAnthropicResponsesPart(item); ok {
			content = append(content, part)
		}
	}

	return content, role, nil
}

func parseAnthropicResponsesContent(raw json.RawMessage) ([]models.ContentPart, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return parseAnthropicResponsesContentValue(value)
}

func parseAnthropicResponsesContentValue(value interface{}) ([]models.ContentPart, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		return []models.ContentPart{{Type: "text", Text: v}}, nil
	case []interface{}:
		out := make([]models.ContentPart, 0, len(v))
		for _, item := range v {
			part, ok := parseAnthropicResponsesPart(item)
			if ok {
				out = append(out, part)
			}
		}
		return out, nil
	default:
		return nil, nil
	}
}

func parseAnthropicResponsesPart(value interface{}) (models.ContentPart, bool) {
	item, ok := value.(map[string]interface{})
	if !ok {
		return models.ContentPart{}, false
	}

	switch stringValue(item["type"]) {
	case "text", "output_text":
		text := textValue(item["text"])
		if text == "" {
			return models.ContentPart{}, false
		}
		return models.ContentPart{Type: "text", Text: text}, true
	case "tool_use":
		return models.ContentPart{
			Type:  "tool_use",
			ID:    stringValue(item["id"]),
			Name:  stringValue(item["name"]),
			Input: item["input"],
		}, true
	case "tool_result":
		return models.ContentPart{
			Type:      "tool_result",
			ToolUseID: stringValue(item["tool_use_id"]),
			Content:   item["content"],
		}, true
	case "thinking":
		return models.ContentPart{
			Type:      "thinking",
			Thinking:  stringValue(item["thinking"]),
			Signature: stringValue(item["signature"]),
		}, true
	default:
		if text := textValue(item["text"]); text != "" {
			return models.ContentPart{Type: "text", Text: text}, true
		}
		return models.ContentPart{}, false
	}
}

func buildAnthropicToolUses(toolCalls []proxyOpenAIChatCall) []models.ContentPart {
	out := make([]models.ContentPart, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, models.ContentPart{
			Type:  "tool_use",
			ID:    defaultToolCallID(call.ID),
			Name:  call.Function.Name,
			Input: parseJSONString(call.Function.Arguments),
		})
	}
	return out
}

func hasOpenAIContent(content interface{}) bool {
	switch v := content.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []map[string]interface{}:
		return len(v) > 0
	case []interface{}:
		return len(v) > 0
	default:
		return true
	}
}

func imageSourceToDataURL(source *models.ImageSource) string {
	if source == nil || source.MediaType == "" || source.Data == "" {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", source.MediaType, source.Data)
}

func stringifyToolResult(content interface{}) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	case []models.ContentPart:
		var textParts []string
		for _, part := range v {
			if part.Type == "text" && part.Text != "" {
				textParts = append(textParts, part.Text)
			}
		}
		if len(textParts) > 0 {
			return strings.Join(textParts, "\n")
		}
	default:
		return marshalJSONValue(v)
	}
	return marshalJSONValue(content)
}

func parseJSONString(raw string) interface{} {
	if raw == "" {
		return map[string]interface{}{}
	}

	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value
	}
	return map[string]interface{}{"raw": raw}
}

func marshalJSONValue(value interface{}) string {
	if value == nil {
		return "{}"
	}

	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func textValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]interface{}:
		if text := stringValue(v["text"]); text != "" {
			return text
		}
		if text := stringValue(v["value"]); text != "" {
			return text
		}
	}
	return ""
}

func stringValue(value interface{}) string {
	v, _ := value.(string)
	return v
}

func defaultToolCallID(id string) string {
	if id != "" {
		return id
	}
	return "tool_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func mapOpenAIFinishReason(reason string) string {
	switch reason {
	case "stop", "":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	default:
		return reason
	}
}

type proxyOpenAIChatStreamResponse struct {
	ID      string                        `json:"id"`
	Model   string                        `json:"model"`
	Choices []proxyOpenAIChatStreamChoice `json:"choices"`
	Usage   *proxyOpenAIUsage             `json:"usage,omitempty"`
}

type proxyOpenAIChatStreamChoice struct {
	Index        int                        `json:"index"`
	Delta        proxyOpenAIChatStreamDelta `json:"delta"`
	FinishReason string                     `json:"finish_reason"`
}

type proxyOpenAIChatStreamDelta struct {
	Role      string                         `json:"role"`
	Content   string                         `json:"content"`
	ToolCalls []proxyOpenAIChatToolCallDelta `json:"tool_calls"`
}

type proxyOpenAIChatToolCallDelta struct {
	Index    int                          `json:"index"`
	ID       string                       `json:"id"`
	Type     string                       `json:"type"`
	Function proxyOpenAIFunctionCallDelta `json:"function"`
}

type proxyOpenAIFunctionCallDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type proxyResponsesStreamEvent struct {
	Type     string                           `json:"type"`
	Delta    string                           `json:"delta"`
	Response *proxyAnthropicResponsesEnvelope `json:"response,omitempty"`
}

type sseTransformer interface {
	Transform(line []byte) ([][]byte, error)
	Finalize() ([][]byte, error)
}

func newSSETransformer(apiType APIType, fallbackModel string) sseTransformer {
	switch apiType {
	case APITypeAnthropicMessages:
		return &passthroughSSETransformer{}
	case APITypeOpenAIChat:
		return newOpenAIChatSSETransformer(fallbackModel)
	case APITypeAnthropicResponses:
		return newResponsesSSETransformer(fallbackModel)
	default:
		return &passthroughSSETransformer{}
	}
}

type passthroughSSETransformer struct{}

func (t *passthroughSSETransformer) Transform(line []byte) ([][]byte, error) {
	if len(line) == 0 {
		return nil, nil
	}
	return [][]byte{append([]byte(nil), line...)}, nil
}

func (t *passthroughSSETransformer) Finalize() ([][]byte, error) {
	return nil, nil
}

type openAIToolCallStreamState struct {
	anthropicIndex int
	id             string
	name           string
	pendingArgs    string
	started        bool
}

type openAIChatSSETransformer struct {
	fallbackModel  string
	messageID      string
	model          string
	messageStarted bool
	textBlockOpen  bool
	textBlockIndex int
	nextIndex      int
	toolCalls      map[int]*openAIToolCallStreamState
	toolOrder      []*openAIToolCallStreamState
	inputTokens    int
	outputTokens   int
	finished       bool
}

func newOpenAIChatSSETransformer(fallbackModel string) *openAIChatSSETransformer {
	return &openAIChatSSETransformer{
		fallbackModel: fallbackModel,
		toolCalls:     make(map[int]*openAIToolCallStreamState),
	}
}

func (t *openAIChatSSETransformer) Transform(line []byte) ([][]byte, error) {
	raw := strings.TrimSpace(string(line))
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "data: ") {
		return nil, nil
	}

	payload := strings.TrimSpace(strings.TrimPrefix(raw, "data: "))
	if payload == "" {
		return nil, nil
	}
	if payload == "[DONE]" {
		return t.finish("stop"), nil
	}

	var chunk proxyOpenAIChatStreamResponse
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return nil, nil
	}

	if chunk.Usage != nil {
		t.inputTokens = chunk.Usage.PromptTokens
		t.outputTokens = chunk.Usage.CompletionTokens
	}

	out := t.ensureMessageStart(chunk.ID, chunk.Model)
	for _, choice := range chunk.Choices {
		if len(choice.Delta.ToolCalls) > 0 {
			out = append(out, t.handleToolCallDeltas(choice.Delta.ToolCalls)...)
		}
		if choice.Delta.Content != "" {
			out = append(out, t.ensureTextBlockStart()...)
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_delta",
				"index": t.textBlockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": choice.Delta.Content,
				},
			}))
		}
		if choice.FinishReason != "" {
			out = append(out, t.finish(choice.FinishReason)...)
		}
	}

	return out, nil
}

func (t *openAIChatSSETransformer) Finalize() ([][]byte, error) {
	if !t.messageStarted || t.finished {
		return nil, nil
	}
	return t.finish("stop"), nil
}

func (t *openAIChatSSETransformer) ensureMessageStart(id, model string) [][]byte {
	if id != "" && t.messageID == "" {
		t.messageID = id
	}
	if t.messageID == "" {
		t.messageID = newProxyMessageID()
	}
	if model != "" {
		t.model = model
	}
	if t.model == "" {
		t.model = t.fallbackModel
	}
	if t.messageStarted {
		return nil
	}
	t.messageStarted = true
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            t.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         t.model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  t.inputTokens,
				"output_tokens": 0,
			},
		},
	})}
}

func (t *openAIChatSSETransformer) ensureTextBlockStart() [][]byte {
	if t.textBlockOpen {
		return nil
	}
	t.textBlockOpen = true
	t.textBlockIndex = t.nextIndex
	t.nextIndex++
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type":  "content_block_start",
		"index": t.textBlockIndex,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	})}
}

func (t *openAIChatSSETransformer) closeTextBlock() [][]byte {
	if !t.textBlockOpen {
		return nil
	}
	t.textBlockOpen = false
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type":  "content_block_stop",
		"index": t.textBlockIndex,
	})}
}

func (t *openAIChatSSETransformer) handleToolCallDeltas(deltas []proxyOpenAIChatToolCallDelta) [][]byte {
	var out [][]byte
	for _, delta := range deltas {
		state, ok := t.toolCalls[delta.Index]
		if !ok {
			state = &openAIToolCallStreamState{anthropicIndex: t.nextIndex}
			t.nextIndex++
			t.toolCalls[delta.Index] = state
			t.toolOrder = append(t.toolOrder, state)
		}

		if delta.ID != "" {
			state.id = delta.ID
		}
		if delta.Function.Name != "" {
			state.name = delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			state.pendingArgs += delta.Function.Arguments
		}

		if !state.started && state.name != "" {
			out = append(out, t.closeTextBlock()...)
			out = append(out, t.startToolState(state)...)
			continue
		}
		if state.started && delta.Function.Arguments != "" {
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_delta",
				"index": state.anthropicIndex,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": delta.Function.Arguments,
				},
			}))
		}
	}
	return out
}

func (t *openAIChatSSETransformer) startToolState(state *openAIToolCallStreamState) [][]byte {
	if state.started {
		return nil
	}
	state.started = true
	state.id = defaultToolCallID(state.id)

	out := [][]byte{marshalSSEEvent(map[string]interface{}{
		"type":  "content_block_start",
		"index": state.anthropicIndex,
		"content_block": map[string]interface{}{
			"type":  "tool_use",
			"id":    state.id,
			"name":  state.name,
			"input": map[string]interface{}{},
		},
	})}
	if state.pendingArgs != "" {
		out = append(out, marshalSSEEvent(map[string]interface{}{
			"type":  "content_block_delta",
			"index": state.anthropicIndex,
			"delta": map[string]interface{}{
				"type":         "input_json_delta",
				"partial_json": state.pendingArgs,
			},
		}))
		state.pendingArgs = ""
	}
	return out
}

func (t *openAIChatSSETransformer) finish(reason string) [][]byte {
	if t.finished {
		return nil
	}
	out := t.ensureMessageStart("", "")
	out = append(out, t.closeTextBlock()...)
	for _, state := range t.toolOrder {
		if !state.started {
			if state.name == "" {
				state.name = "tool"
			}
			out = append(out, t.startToolState(state)...)
		}
		out = append(out, marshalSSEEvent(map[string]interface{}{
			"type":  "content_block_stop",
			"index": state.anthropicIndex,
		}))
	}
	out = append(out, marshalSSEEvent(map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   mapOpenAIFinishReason(reason),
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"input_tokens":  t.inputTokens,
			"output_tokens": t.outputTokens,
		},
	}))
	out = append(out, marshalSSEEvent(map[string]interface{}{"type": "message_stop"}))
	t.finished = true
	return out
}

type responsesSSETransformer struct {
	fallbackModel  string
	messageID      string
	model          string
	messageStarted bool
	textBlockOpen  bool
	textBlockIndex int
	nextIndex      int
	inputTokens    int
	outputTokens   int
	finished       bool
}

func newResponsesSSETransformer(fallbackModel string) *responsesSSETransformer {
	return &responsesSSETransformer{fallbackModel: fallbackModel}
}

func (t *responsesSSETransformer) Transform(line []byte) ([][]byte, error) {
	raw := strings.TrimSpace(string(line))
	if raw == "" {
		return nil, nil
	}
	if !strings.HasPrefix(raw, "data: ") {
		return nil, nil
	}

	payload := strings.TrimSpace(strings.TrimPrefix(raw, "data: "))
	if payload == "" || payload == "[DONE]" {
		return t.finish("end_turn"), nil
	}

	var event proxyResponsesStreamEvent
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		return nil, nil
	}

	t.captureResponseEnvelope(event.Response)
	out := [][]byte(nil)
	switch event.Type {
	case "response.created", "response.in_progress":
		out = append(out, t.ensureMessageStart()...)
	case "response.output_text.delta":
		out = append(out, t.ensureMessageStart()...)
		out = append(out, t.ensureTextBlockStart()...)
		if event.Delta != "" {
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_delta",
				"index": t.textBlockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": event.Delta,
				},
			}))
		}
	case "response.completed":
		out = append(out, t.ensureMessageStart()...)
		if event.Response != nil {
			parts, err := parseContentPartsFromResponsesEnvelope(event.Response)
			if err != nil {
				return nil, err
			}
			out = append(out, t.emitCompletedContent(parts)...)
			stopReason := event.Response.StopReason
			if stopReason == "" {
				stopReason = "end_turn"
			}
			out = append(out, t.finish(stopReason)...)
			return out, nil
		}
		out = append(out, t.finish("end_turn")...)
	}
	return out, nil
}

func (t *responsesSSETransformer) Finalize() ([][]byte, error) {
	if !t.messageStarted || t.finished {
		return nil, nil
	}
	return t.finish("end_turn"), nil
}

func (t *responsesSSETransformer) captureResponseEnvelope(resp *proxyAnthropicResponsesEnvelope) {
	if resp == nil {
		return
	}
	if resp.ID != "" {
		t.messageID = resp.ID
	}
	if resp.Model != "" {
		t.model = resp.Model
	}
	t.inputTokens = resp.Usage.InputTokens
	t.outputTokens = resp.Usage.OutputTokens
}

func (t *responsesSSETransformer) ensureMessageStart() [][]byte {
	if t.messageID == "" {
		t.messageID = newProxyMessageID()
	}
	if t.model == "" {
		t.model = t.fallbackModel
	}
	if t.messageStarted {
		return nil
	}
	t.messageStarted = true
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            t.messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         t.model,
			"content":       []interface{}{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  t.inputTokens,
				"output_tokens": 0,
			},
		},
	})}
}

func (t *responsesSSETransformer) ensureTextBlockStart() [][]byte {
	if t.textBlockOpen {
		return nil
	}
	t.textBlockOpen = true
	t.textBlockIndex = t.nextIndex
	t.nextIndex++
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type":  "content_block_start",
		"index": t.textBlockIndex,
		"content_block": map[string]interface{}{
			"type": "text",
			"text": "",
		},
	})}
}

func (t *responsesSSETransformer) closeTextBlock() [][]byte {
	if !t.textBlockOpen {
		return nil
	}
	t.textBlockOpen = false
	return [][]byte{marshalSSEEvent(map[string]interface{}{
		"type":  "content_block_stop",
		"index": t.textBlockIndex,
	})}
}

func (t *responsesSSETransformer) emitCompletedContent(parts []models.ContentPart) [][]byte {
	var out [][]byte
	for _, part := range parts {
		switch part.Type {
		case "text":
			if part.Text == "" {
				continue
			}
			out = append(out, t.ensureTextBlockStart()...)
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_delta",
				"index": t.textBlockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": part.Text,
				},
			}))
		case "tool_use":
			out = append(out, t.closeTextBlock()...)
			index := t.nextIndex
			t.nextIndex++
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_start",
				"index": index,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    defaultToolCallID(part.ID),
					"name":  part.Name,
					"input": part.Input,
				},
			}))
			out = append(out, marshalSSEEvent(map[string]interface{}{
				"type":  "content_block_stop",
				"index": index,
			}))
		}
	}
	return out
}

func (t *responsesSSETransformer) finish(stopReason string) [][]byte {
	if t.finished {
		return nil
	}
	out := t.ensureMessageStart()
	out = append(out, t.closeTextBlock()...)
	out = append(out, marshalSSEEvent(map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"input_tokens":  t.inputTokens,
			"output_tokens": t.outputTokens,
		},
	}))
	out = append(out, marshalSSEEvent(map[string]interface{}{"type": "message_stop"}))
	t.finished = true
	return out
}

func parseContentPartsFromResponsesEnvelope(envelope *proxyAnthropicResponsesEnvelope) ([]models.ContentPart, error) {
	if envelope == nil {
		return nil, nil
	}

	content, _, err := parseAnthropicResponsesOutput(envelope.Output)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		content, err = parseAnthropicResponsesContent(envelope.Content)
		if err != nil {
			return nil, err
		}
	}
	if len(content) == 0 && envelope.OutputText != "" {
		content = []models.ContentPart{{Type: "text", Text: envelope.OutputText}}
	}
	return content, nil
}

func marshalSSEEvent(payload interface{}) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return []byte("data: " + string(data) + "\n\n")
}

func newProxyMessageID() string {
	return "msg_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

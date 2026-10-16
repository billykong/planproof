package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Local is a planner on an OpenAI-compatible chat API with tool calls, as
// the safe-ops local model server (vLLM) serves (E-03). Plain HTTP: the
// Anthropic SDK is for Claude only.
type Local struct {
	BaseURL string // e.g. http://172.30.99.10:8000/v1
	Key     string // bearer key
	Model   string // served model name
	HTTP    *http.Client
}

func (l *Local) Name() string { return l.Model }

func (l *Local) Start(system, user string) Session {
	return &localSession{l: l, messages: []map[string]any{
		{"role": "system", "content": system},
		{"role": "user", "content": user},
	}}
}

type localSession struct {
	l        *Local
	messages []map[string]any
}

func (s *localSession) Answer(callID, content string) {
	s.messages = append(s.messages, map[string]any{"role": "tool", "tool_call_id": callID, "content": content})
}

func (s *localSession) Say(text string) {
	s.messages = append(s.messages, map[string]any{"role": "user", "content": text})
}

func (s *localSession) Next(ctx context.Context) (Turn, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       s.l.Model,
		"messages":    s.messages,
		"temperature": 0, // repeatable runs
		"max_tokens":  2048,
		"tools": []any{map[string]any{"type": "function", "function": map[string]any{
			"name": ToolName, "description": ToolDescription, "parameters": toolSchemaMap()}}},
		"tool_choice": "auto",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.l.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Turn{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.l.Key)
	client := s.l.HTTP
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Turn{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return Turn{}, fmt.Errorf("local model %d: %s", resp.StatusCode, bytes.TrimSpace(raw[:min(len(raw), 500)]))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return Turn{}, fmt.Errorf("local model: unreadable response: %v", err)
	}
	m := out.Choices[0].Message
	turn := Turn{Text: m.Content, Usage: Usage{Input: out.Usage.PromptTokens, Output: out.Usage.CompletionTokens}}
	// History is append-only, as the model returned it.
	assistant := map[string]any{"role": "assistant", "content": m.Content}
	if len(m.ToolCalls) > 0 {
		assistant["tool_calls"] = m.ToolCalls
	}
	s.messages = append(s.messages, assistant)
	for _, c := range m.ToolCalls {
		if c.Function.Name == ToolName && turn.Call == nil {
			turn.Call = &Call{ID: c.ID, Input: json.RawMessage(c.Function.Arguments)}
		} else {
			s.Answer(c.ID, "error: one submit_plan call per turn")
		}
	}
	if turn.Call == nil && out.Choices[0].FinishReason == "length" {
		return turn, fmt.Errorf("the model ran out of output tokens")
	}
	return turn, nil
}

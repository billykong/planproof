package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// Claude is a planner on the Anthropic API (E-03). The client reads its
// credentials the SDK's usual way (ANTHROPIC_API_KEY, or a profile).
type Claude struct {
	Client anthropic.Client
	Model  string // e.g. "claude-opus-5-5"
	Effort anthropic.BetaOutputConfigEffort
	Label  string // optional, added to Name (e.g. a prompt version)
}

// fallbackModels support server-side refusal fallbacks (Claude Haiku 5.5
// doesn't: a refusal there simply stops).
var fallbackModels = map[string]bool{"claude-fable-5-1": true, "claude-opus-5-5": true, "claude-opus-5": true, "claude-sonnet-5-5": true}

// NewClaude makes a planner on model; opts go to the client.
func NewClaude(model string, effort anthropic.BetaOutputConfigEffort, opts ...option.RequestOption) *Claude {
	return &Claude{Client: anthropic.NewClient(opts...), Model: model, Effort: effort}
}

func (c *Claude) Name() string {
	if c.Label != "" {
		return c.Model + "/" + string(c.Effort) + "/" + c.Label
	}
	return c.Model + "/" + string(c.Effort)
}

func (c *Claude) Start(system, user string) Session {
	return &claudeSession{c: c, system: system, messages: []anthropic.BetaMessageParam{
		// The task is the large, stable part: cached, so each repair pays
		// for the snapshot once (verify with CacheReadInputTokens).
		{Role: anthropic.BetaMessageParamRoleUser, Content: []anthropic.BetaContentBlockParamUnion{{
			OfText: &anthropic.BetaTextBlockParam{Text: user, CacheControl: anthropic.NewBetaCacheControlEphemeralParam()},
		}}},
	}}
}

type claudeSession struct {
	c        *Claude
	system   string
	messages []anthropic.BetaMessageParam
	pending  []anthropic.BetaContentBlockParamUnion
}

func (s *claudeSession) Answer(callID, content string) {
	s.pending = append(s.pending, anthropic.NewBetaToolResultBlock(callID, content, false))
}

func (s *claudeSession) Say(text string) {
	s.pending = append(s.pending, anthropic.NewBetaTextBlock(text))
}

func (s *claudeSession) Next(ctx context.Context) (Turn, error) {
	if len(s.pending) > 0 {
		s.messages = append(s.messages, anthropic.NewBetaUserMessage(s.pending...))
		s.pending = nil
	}
	tool := anthropic.BetaToolParam{
		Name:        ToolName,
		Description: anthropic.String(ToolDescription),
		InputSchema: anthropic.BetaToolInputSchemaParam{Properties: toolSchemaMap()["properties"], Required: []string{"goal", "steps"}},
	}
	adaptive := anthropic.BetaThinkingConfigAdaptiveParam{}
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(s.c.Model),
		MaxTokens: 16000,
		System: []anthropic.BetaTextBlockParam{{
			Text: s.system, CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		}},
		Messages:     s.messages,
		Tools:        []anthropic.BetaToolUnionParam{{OfTool: &tool}},
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &adaptive},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: s.c.Effort},
	}
	if fallbackModels[s.c.Model] {
		// A refusal is re-served by a fallback model in the same call.
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
	}
	resp, err := s.c.Client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return Turn{}, fmt.Errorf("anthropic API %d: %s", apiErr.StatusCode, apiErr.Error())
		}
		return Turn{}, err
	}
	u := resp.Usage
	turn := Turn{Usage: Usage{Input: u.InputTokens, Output: u.OutputTokens, CacheRead: u.CacheReadInputTokens, CacheWrite: u.CacheCreationInputTokens}}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return turn, fmt.Errorf("the model refused: %s", resp.StopDetails.Category)
	}
	// History is append-only: the response goes back unchanged, thinking
	// blocks included.
	s.messages = append(s.messages, resp.ToParam())
	for _, b := range resp.Content {
		switch v := b.AsAny().(type) {
		case anthropic.BetaTextBlock:
			turn.Text += v.Text
		case anthropic.BetaToolUseBlock:
			if v.Name == ToolName && turn.Call == nil {
				turn.Call = &Call{ID: v.ID, Input: []byte(v.JSON.Input.Raw())}
			} else {
				// One plan per turn: anything else gets an error result.
				s.pending = append(s.pending, anthropic.NewBetaToolResultBlock(v.ID, "one submit_plan call per turn", true))
			}
		}
	}
	if turn.Call == nil && resp.StopReason == anthropic.BetaStopReasonMaxTokens {
		return turn, errors.New("the model ran out of output tokens")
	}
	return turn, nil
}

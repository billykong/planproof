package agent

import (
	"context"
	"encoding/json"
	"fmt"
)

// Scripted is a model that replays fixed plan bodies, then a final text:
// the loop and the grading, tested without a network or a bill (E-03).
type Scripted struct {
	Plans []json.RawMessage
	Final string
}

func (s Scripted) Name() string { return "scripted" }

func (s Scripted) Start(system, user string) Session { return &scriptedSession{s: s} }

type scriptedSession struct {
	s Scripted
	i int
}

func (ss *scriptedSession) Next(ctx context.Context) (Turn, error) {
	if ss.i < len(ss.s.Plans) {
		ss.i++
		return Turn{Call: &Call{ID: fmt.Sprintf("call-%d", ss.i), Input: ss.s.Plans[ss.i-1]}}, nil
	}
	return Turn{Text: ss.s.Final}, nil
}

func (ss *scriptedSession) Answer(callID, content string) {}

func (ss *scriptedSession) Say(text string) {}

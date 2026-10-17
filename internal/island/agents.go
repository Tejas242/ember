package island

import (
	"sort"
	"time"
)

// Subagents. Claude Code tags every hook a subagent fires with its agent_id,
// so its tool calls are its own story, not the session's: they must not
// rewrite the session's label or its timeline. Agents also outlive the turn
// that launched them (they run in the background), so a session whose turn
// has ended is still busy while one runs.

type Agent struct {
	ID, Type    string
	Desc        string // what it was asked to do, from the Agent call that launched it
	Label       string // what it's doing now
	Start, Seen time.Time
	Steps       int
	Hist        []Step    // its last few steps, newest last: its short transcript
	End         time.Time // when it finished (finished agents only)
	Summary     string    // its own last word, summarized (finished agents only)
}

const (
	agentHist    = 4 // steps an agent keeps
	finishedKept = 3 // finished agents a session keeps, this turn
)

// note records a step the agent took; the one before it is over.
func (ag *Agent) note(now time.Time, tool string, tin map[string]any) {
	if n := len(ag.Hist); n > 0 {
		ag.Hist[n-1].Done = true
	}
	kind, text := pastStep(tool, tin)
	_, plus, minus := activity(tool, tin)
	ag.Hist = append(ag.Hist, Step{At: now, Kind: kind, Text: text, Plus: plus, Minus: minus})
	if len(ag.Hist) > agentHist {
		ag.Hist = ag.Hist[len(ag.Hist)-agentHist:]
	}
}

type agentAsk struct{ typ, desc string }

// running is how many subagents this session has going.
func (s *Session) running() int { return len(s.Agents) }

// status is State, except that a session whose turn ended while its agents
// work on is working too.
func (s *Session) status() string {
	if s.State == "idle" && len(s.Agents) > 0 {
		return "work"
	}
	return s.State
}

// agentList: the running agents, longest-running first.
func (s *Session) agentList() []*Agent {
	out := make([]*Agent, 0, len(s.Agents))
	for _, a := range s.Agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// asked remembers an Agent call's description until its SubagentStart comes.
func (s *Session) asked(tin map[string]any) {
	typ, _ := tin["subagent_type"].(string)
	desc, _ := tin["description"].(string)
	if typ == "" {
		typ = "general-purpose"
	}
	s.asks = append(s.asks, agentAsk{typ, desc})
	if len(s.asks) > 8 {
		s.asks = s.asks[len(s.asks)-8:]
	}
}

// claim takes the oldest waiting description for an agent of this type.
func (s *Session) claim(typ string) string {
	for i, a := range s.asks {
		if a.typ == typ || typ == "" {
			s.asks = append(s.asks[:i:i], s.asks[i+1:]...)
			return a.desc
		}
	}
	return ""
}

// agent finds or starts the record for an agent; nil if it already ended
// (async hooks can arrive after the agent's SubagentStop).
func (s *Session) agent(id, typ string, now time.Time) *Agent {
	if _, gone := s.ended[id]; gone {
		return nil
	}
	if s.Agents == nil {
		s.Agents = map[string]*Agent{}
	}
	ag := s.Agents[id]
	if ag == nil {
		ag = &Agent{ID: id, Type: typ, Desc: s.claim(typ), Start: now}
		s.Agents[id] = ag
	}
	if ag.Type == "" {
		ag.Type = typ
	}
	ag.Seen = now
	return ag
}

// agentHook takes the events a subagent fires. It reports whether it took
// the event; requests (permissions, questions) go on to the session.
func (a *app) agentHook(s *Session, event string, in hookInput, tin map[string]any, now time.Time) bool {
	switch event {
	case "SubagentStart":
		s.agent(in.AgentID, in.AgentType, now)
	case "SubagentStop":
		if ag, ok := s.Agents[in.AgentID]; ok {
			delete(s.Agents, in.AgentID)
			if n := len(ag.Hist); n > 0 {
				ag.Hist[n-1].Done = true
			}
			ag.End, ag.Summary = now, summarize(in.LastMessage)
			s.Finished = append(s.Finished, ag)
			if len(s.Finished) > finishedKept {
				s.Finished = s.Finished[len(s.Finished)-finishedKept:]
			}
		} else {
			s.ended = remember(s.ended, in.AgentID, now)
		}
	case "PreToolUse":
		if ag := s.agent(in.AgentID, in.AgentType, now); ag != nil {
			ag.Label, _, _ = activity(in.Tool, tin)
			ag.Steps++
			ag.note(now, in.Tool, tin)
		}
		a.ui.day.call(now)
	case "PostToolUse", "PostToolUseFailure":
		if ag := s.agent(in.AgentID, in.AgentType, now); ag != nil {
			if n := len(ag.Hist); n > 0 && !ag.Hist[n-1].Done {
				ag.Hist[n-1].Done, ag.Hist[n-1].Failed = true, event == "PostToolUseFailure"
			}
		}
	default:
		return false
	}
	return true
}

// remember adds id to a small set of ended agents, forgetting old ones.
func remember(m map[string]time.Time, id string, now time.Time) map[string]time.Time {
	if m == nil {
		m = map[string]time.Time{}
	}
	for k, t := range m {
		if now.Sub(t) > time.Hour {
			delete(m, k)
		}
	}
	m[id] = now
	return m
}

// sweepAgents drops agents that went silent: one that was killed (Esc, a
// crash) never says SubagentStop.
func (s *Session) sweepAgents(now time.Time, quiet time.Duration) bool {
	changed := false
	for id, ag := range s.Agents {
		if now.Sub(ag.Seen) > quiet {
			delete(s.Agents, id)
			changed = true
		}
	}
	return changed
}

// agentName is how an agent is named in a row: its task, else its kind.
func (ag *Agent) name() string {
	if ag.Desc != "" {
		return ag.Desc
	}
	switch ag.Type {
	case "", "general-purpose":
		return "Agent"
	}
	return ag.Type
}

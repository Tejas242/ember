package island

import "time"

// Subagents. Claude Code tags every hook a subagent fires with its agent_id,
// so its tool calls are its own, not the session's. Agents also outlive the
// turn that launched them (they run in the background), so a session whose
// turn has ended is still busy while one runs. Ember counts them; it keeps
// nothing else about them.

type Agent struct {
	ID, Type    string
	Start, Seen time.Time
}

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
		ag = &Agent{ID: id, Type: typ, Start: now}
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
func (s *Session) agentHook(event string, in hookInput, now time.Time) bool {
	switch event {
	case "SubagentStart":
		s.agent(in.AgentID, in.AgentType, now)
	case "SubagentStop":
		if _, ok := s.Agents[in.AgentID]; ok {
			delete(s.Agents, in.AgentID)
		} else {
			s.ended = remember(s.ended, in.AgentID, now)
		}
	case "PostToolUse": // it is still at work: it isn't taken for gone
		s.agent(in.AgentID, in.AgentType, now)
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

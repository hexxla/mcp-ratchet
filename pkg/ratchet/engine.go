package ratchet

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
	"uuid"
)

const (
	maxCompletionsPerTool = 64
	maxSessions           = 1024
	idleTimeout           = 30 * time.Minute
	sweepPeriod           = time.Minute
)

// Options configures optional observation. Observer is called synchronously
// outside global bookkeeping locks and must be safe for concurrent calls. It
// must return promptly and must not reenter this engine for the same session.
// A nil Observer disables event creation entirely.
type Options struct {
	Observer func(Event)
}

// Engine enforces a fixed policy with process-local completion state. Use New;
// the zero value is not initialized. Engines are safe for concurrent use.
// One engine must own each session's workflow. Session identities and resource
// authorization are the embedding application's responsibility.
type Engine struct {
	byTool      map[string][]Rule
	producerTTL map[string]time.Duration
	observer    func(Event)
	now         func() time.Time
	mu          sync.Mutex
	sessions    map[string]*session
	nextSweep   time.Time
}

type session struct {
	gate        chan struct{}
	users       int
	idleAt      time.Time
	completions map[string][]time.Time
}

// New validates and clones policy before creating an engine. Subsequent changes
// to policy or its nested AfterAny slices cannot change the engine's behavior.
func New(policy Policy, options Options) (*Engine, error) {
	byTool, producerTTL, err := compilePolicy(policy)
	if err != nil {
		return nil, err
	}
	return &Engine{
		byTool: byTool, producerTTL: producerTTL, observer: options.Observer,
		now: time.Now, sessions: make(map[string]*session),
	}, nil
}

// ExecuteToolCall serializes a session, checks alternatives, and consumes the
// selected completion before running the call. A false result or an error spends
// the reservation and records no completion. The handler's error is preserved;
// false with no error returns nil so transports can retain their own error result.
// Only tools used as prerequisites produce completions. Queued calls honor ctx.
func (e *Engine) ExecuteToolCall(ctx context.Context, sessionID, tool string, run func() (bool, error)) error {
	if sessionID == "" || tool == "" || run == nil {
		return fmt.Errorf("%w: session ID, tool and handler are required", ErrInvalidCall)
	}
	entry, release, err := e.acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return err
	}
	e.emit(Event{Type: EventTypeToolCallAttempt, SessionID: sessionID, ToolName: tool})
	// An observer may delay the attempt; evaluate expiry immediately before
	// selecting and consuming its prerequisite, rather than before the callback.
	entry.prune(e.now())
	hadCompletions := len(entry.completions) != 0
	prerequisite, consume, err := e.selectPrerequisite(entry, tool)
	if err != nil {
		e.emit(Event{Type: EventTypeToolCallFailure, SessionID: sessionID, ToolName: tool, Error: err.Error()})
		return err
	}
	if consume {
		values := entry.completions[prerequisite]
		expires := values[len(values)-1]
		entry.set(prerequisite, slices.Delete(values, len(values)-1, len(values)))
		e.emit(Event{Type: EventTypeCompletionConsumed, SessionID: sessionID, ToolName: prerequisite, ExpiresAt: expires, ConsumedBy: tool})
	}
	ok, err := run()
	if err != nil || !ok {
		event := Event{Type: EventTypeToolCallFailure, SessionID: sessionID, ToolName: tool}
		if err != nil {
			event.Error = err.Error()
		}
		e.emit(event)
		return err
	}
	if ttl, tracked := e.producerTTL[tool]; tracked {
		expires := e.now().Add(ttl)
		values := entry.completions[tool]
		if len(values) == maxCompletionsPerTool {
			values = slices.Delete(values, 0, 1)
		}
		entry.set(tool, append(values, expires))
		if !hadCompletions {
			e.emit(Event{Type: EventTypeSessionCreated, SessionID: sessionID})
		}
		e.emit(Event{Type: EventTypeCompletionRecorded, SessionID: sessionID, ToolName: tool, ExpiresAt: expires})
	}
	e.emit(Event{Type: EventTypeToolCallSuccess, SessionID: sessionID, ToolName: tool})
	return nil
}

func (e *Engine) selectPrerequisite(entry *session, tool string) (string, bool, error) {
	rules := e.byTool[tool]
	if len(rules) == 0 || len(rules[0].AfterAny) == 0 {
		return "", false, nil
	}
	var last Rule
	for _, rule := range rules {
		for _, prerequisite := range rule.AfterAny {
			if len(entry.completions[prerequisite]) != 0 {
				return prerequisite, rule.Consume, nil
			}
		}
		last = rule
	}
	message := last.Message
	if message == "" {
		message = fmt.Sprintf("tool %q requires a successful prerequisite: %v", tool, last.AfterAny)
	}
	return "", false, fmt.Errorf("%w: %s", ErrPrerequisiteRequired, message)
}

func (e *Engine) emit(event Event) {
	if e.observer == nil {
		return
	}
	event.ID = uuid.NewV4().String()
	event.Timestamp = e.now()
	e.observer(event)
}

func (e *Engine) acquire(ctx context.Context, id string) (*session, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	now := e.now()
	e.mu.Lock()
	entry := e.sessions[id]
	if !now.Before(e.nextSweep) || (entry == nil && len(e.sessions) >= maxSessions) ||
		(entry != nil && entry.users == 0 && !now.Before(entry.idleAt.Add(idleTimeout))) {
		// All storage is engine-owned memory. Idle entries have no users, so
		// reclamation needs neither external calls nor another lock or pin.
		for key, candidate := range e.sessions {
			if candidate.users != 0 {
				continue
			}
			candidate.prune(now)
			if len(candidate.completions) == 0 || !now.Before(candidate.idleAt.Add(idleTimeout)) {
				delete(e.sessions, key)
			}
		}
		e.nextSweep = now.Add(sweepPeriod)
	}
	entry = e.sessions[id]
	if entry == nil {
		if len(e.sessions) >= maxSessions {
			e.mu.Unlock()
			return nil, nil, ErrSessionLimit
		}
		entry = &session{gate: make(chan struct{}, 1), completions: make(map[string][]time.Time)}
		e.sessions[id] = entry
	}
	entry.users++
	e.mu.Unlock()
	select {
	case entry.gate <- struct{}{}:
		return entry, func() {
			now := e.now()
			entry.prune(now)
			e.mu.Lock()
			e.releaseUser(id, entry, now)
			e.mu.Unlock()
			<-entry.gate
		}, nil
	case <-ctx.Done():
		now := e.now()
		e.mu.Lock()
		e.releaseUser(id, entry, now)
		e.mu.Unlock()
		return nil, nil, ctx.Err()
	}
}

// releaseUser runs with mu held. Queued callers pin the same entry until the last
// caller leaves, preventing cleanup from splitting one session into two gates.
func (e *Engine) releaseUser(id string, entry *session, now time.Time) {
	entry.users--
	if entry.users == 0 {
		if len(entry.completions) == 0 {
			delete(e.sessions, id)
		} else {
			entry.idleAt = now
		}
	}
}

func (s *session) prune(now time.Time) {
	for tool, values := range s.completions {
		s.set(tool, slices.DeleteFunc(values, func(expires time.Time) bool { return !now.Before(expires) }))
	}
}

func (s *session) set(tool string, values []time.Time) {
	if len(values) == 0 {
		delete(s.completions, tool)
	} else {
		s.completions[tool] = values
	}
}

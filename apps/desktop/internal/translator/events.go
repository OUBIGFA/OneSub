package translator

import (
	"sort"
	"time"
)

// emit sends an event with the run's stage and usage. Callers hold e.mu,
// so events leave in the order they happen.
func (e *engine) emit(ev Event) {
	if e.config.Observe == nil {
		return
	}
	ev.Stage, ev.Done, ev.Total = e.stageNow.Stage, e.stageNow.Done, e.stageNow.Total
	ev.Requests, ev.Input, ev.Output = e.svc.usage()
	e.config.Observe(ev)
}

func (e *engine) stage(name string, done, total int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stageNow = Event{Stage: name, Done: done, Total: total}
	e.emit(Event{})
}

func (e *engine) retryNote(note string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.emit(Event{Retry: note})
}

// update shows a unit's text. Streamed text is sent at most ten times a
// second; accepted text at once.
func (e *engine) update(index int, text string, final bool, thinking string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pending == nil {
		e.pending = map[int]UnitText{}
	}
	if index >= 0 {
		if final {
			e.accepted[index] = text
			e.stageNow.Done = len(e.accepted)
		}
		e.pending[index] = UnitText{Index: index, Text: text, Final: final}
	}
	if !final && time.Since(e.flushed) < 100*time.Millisecond {
		return
	}
	updates := make([]UnitText, 0, len(e.pending))
	for _, u := range e.pending {
		updates = append(updates, u)
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Index < updates[j].Index })
	e.pending = map[int]UnitText{}
	e.flushed = time.Now()
	e.emit(Event{Updates: updates, Thinking: thinking})
}

// Do not lose the final throttled fragment when a request stops suddenly.
func (e *engine) flushUpdates() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.pending) == 0 {
		return
	}
	updates := make([]UnitText, 0, len(e.pending))
	for _, u := range e.pending {
		updates = append(updates, u)
	}
	sort.Slice(updates, func(i, j int) bool { return updates[i].Index < updates[j].Index })
	e.pending = nil
	e.emit(Event{Updates: updates})
}

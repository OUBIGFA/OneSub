package translator

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// translateAll translates the units in todo, in batches sent concurrently.
// It returns the units that stayed untranslated after every correction.
func (e *engine) translateAll(ctx context.Context, units []subtitle.Cue, todo []int, shared any) ([]int, error) {
	if len(todo) == 0 || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	batchSize := e.config.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	batches := [][]int{}
	for start := 0; start < len(todo); {
		end, chars := start, 0
		for end < len(todo) && end-start < batchSize {
			n := len(units[todo[end]].Text)
			if end > start && chars+n > batchChars {
				break
			}
			chars += n
			end++
		}
		// A full batch ends where a sentence does, so one sentence is not
		// translated half in each of two requests. The last quarter of the
		// batch is searched, the latest end first; with none, the cut stays.
		if end < len(todo) {
			for cut := end; cut > start+max(1, (end-start)*3/4); cut-- {
				if sentenceEnd(units[todo[cut-1]].Text) {
					end = cut
					break
				}
			}
		}
		batches = append(batches, todo[start:end])
		start = end
	}
	contextKey := cacheKey(e.key, "context", shared)
	var (
		mu       sync.Mutex
		failed   []int
		firstErr error
		wg       sync.WaitGroup
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	work := make(chan []int)
	// Concurrency is a ceiling on actual batches, not a reason to create idle
	// workers. Large saved limits must stay cheap for short subtitles/resumes.
	for range min(e.concurrency(), len(batches)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range work {
				left, err := e.translateBatch(ctx, units, batch, shared, contextKey)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = err
					cancel()
				}
				failed = append(failed, left...)
				mu.Unlock()
			}
		}()
	}
	for _, batch := range batches {
		select {
		case work <- batch:
		case <-ctx.Done():
		}
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Ints(failed)
	return failed, nil
}

// translateBatch translates one batch. Units an answer misses, leaves empty,
// strips of markup or leaves in the source language are asked for again
// with the problem named, as VideoCaptioner's correction loop does; what is
// still missing then is asked for one subtitle at a time, as SmartSub's
// repair does. It returns the units that never got a valid translation.
func (e *engine) translateBatch(ctx context.Context, units []subtitle.Cue, batch []int, shared any, contextKey string) ([]int, error) {
	pending, problems := []int{}, []string{}
	for _, i := range batch {
		e.mu.Lock()
		_, already := e.accepted[i]
		e.mu.Unlock()
		if already {
			continue
		}
		var text string
		if e.cache.get(cacheKey(contextKey, unitID(i), units[i].Text), &text) {
			e.mu.Lock()
			e.report.Resumed++
			e.mu.Unlock()
			e.update(i, text, true, "")
			continue
		}
		pending = append(pending, i)
	}
	for round := 0; round < 3 && len(pending) > 0; round++ {
		groups := [][]int{pending}
		if round == 2 {
			groups = nil
			for _, i := range pending {
				groups = append(groups, []int{i})
			}
		}
		next, issues := []int{}, []string{}
		for _, group := range groups {
			left, found, err := e.translateUnits(ctx, units, group, shared, contextKey, problems, round)
			if err != nil {
				return nil, err
			}
			next = append(next, left...)
			issues = append(issues, found...)
		}
		pending, problems = next, issues
	}
	return pending, nil
}

// translateUnits sends one request for group and accepts each valid
// translation in its answer. It returns the units still to do and what was
// wrong with them.
func (e *engine) translateUnits(ctx context.Context, units []subtitle.Cue, group []int, shared any, contextKey string, problems []string, round int) ([]int, []string, error) {
	ids := map[string]int{}
	payload := translatePayload{TargetLanguage: targetName(e.lang, e.config.TargetLanguage), LockedGlossary: strings.TrimSpace(e.config.Glossary), Context: shared, PreviousProblems: problems}
	properties := map[string]any{}
	required := []string{}
	for _, i := range group {
		id := unitID(i)
		ids[id] = i
		payload.Units = append(payload.Units, unitLine{ID: id, Text: units[i].Text})
		properties[id] = map[string]any{"type": "string"}
		required = append(required, id)
	}
	// Neighbors give the batch its surroundings; those before it carry the
	// translations already accepted, for continuity.
	e.mu.Lock()
	for i := max(0, group[0]-neighbors); i < min(len(units), group[len(group)-1]+neighbors+1); i++ {
		if _, inGroup := ids[unitID(i)]; inGroup || units[i].ReadOnly {
			continue
		}
		line := unitLine{ID: unitID(i), Text: units[i].Text}
		if i < group[0] {
			line.Translation = e.accepted[i]
		}
		payload.Neighbors = append(payload.Neighbors, line)
	}
	e.mu.Unlock()
	system, err := e.system("translate", translateOperation)
	if err != nil {
		return nil, nil, err
	}
	schema := map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	r := request{system: system, messages: []message{{"user", marshal(payload)}}, schema: schema}
	finished := map[int]string{}
	got, err := e.svc.call(ctx, r, func(content, thinking string) {
		pairs, partial := scanPairs(stripThinking(content))
		shown := false
		for _, p := range pairs {
			if i, ok := ids[p.key]; ok {
				text := strings.TrimSpace(p.value)
				valid := text != "" && sameMarkup(units[i].Text, text) && (round >= 2 || !untranslated(units[i].Text, text, e.lang)) && (round > 0 || len(missingNumbers(units[i].Text, text)) == 0)
				if valid && finished[i] != text {
					finished[i] = text
					e.keep(cacheKey(contextKey, p.key, units[i].Text), text)
					e.update(i, text, true, thinking)
				} else if !valid {
					e.update(i, text, false, thinking)
				}
				shown = true
			}
		}
		if partial != nil {
			if i, ok := ids[partial.key]; ok {
				e.update(i, partial.value, false, thinking)
				shown = true
			}
		}
		if !shown {
			e.update(-1, "", false, thinking)
		}
	})
	if err != nil && got.text == "" {
		return nil, nil, err
	}
	values, parseErr := parseObject(got.text)
	left, issues := []int{}, []string{}
	if parseErr != nil {
		issues = append(issues, "The previous answer was not a JSON object of unit IDs and translations.")
	}
	for _, i := range group {
		id := unitID(i)
		text, ok := values[id]
		text = strings.TrimSpace(text)
		problem := ""
		switch {
		case !ok && got.truncated:
			problem = "unit " + id + " is missing: the answer was cut off"
		case !ok:
			problem = "unit " + id + " is missing"
		case text == "":
			problem = "unit " + id + " is empty"
		case !sameMarkup(units[i].Text, text):
			problem = "unit " + id + " must keep its markup tags " + strings.Join(markupTags.FindAllString(units[i].Text, -1), " ") + " in order"
		case round < 2 && untranslated(units[i].Text, text, e.lang):
			problem = "unit " + id + " is still in the source language: translate it"
		case round == 0 && len(missingNumbers(units[i].Text, text)) > 0:
			// Asked again once with the numbers named; a later answer is
			// accepted and the run warns, as "1st" may rightly become a word.
			problem = "unit " + id + " dropped the source numbers " + strings.Join(missingNumbers(units[i].Text, text), ", ") + ": keep them, written as digits"
		}
		if problem != "" {
			left = append(left, i)
			issues = append(issues, problem)
			continue
		}
		e.keep(cacheKey(contextKey, id, units[i].Text), text)
		e.update(i, text, true, "")
	}
	return left, issues, err
}

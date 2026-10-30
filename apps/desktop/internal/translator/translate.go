// Package translator is OneSub's translation step. The subtitle skill
// (srt-subtitle-translator) is its core: the skill's own files are the
// rules, its repair-translate-divide approach rebuilds the timeline by sense
// groups, and its checker verifies the result. How a model is asked, and
// how its answers are read, checked and asked again, follows VideoCaptioner
// and SmartSub: batches with their context, streamed answers, structured
// output, tolerant parsing, correction requests and repair of single
// subtitles. The model never writes a time: every time comes from the
// source.
package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type Config struct {
	Endpoint, Model, TargetLanguage, SkillRoot string
	Key                                        string `json:"-"`
	// API is APIChatCompletions, the default when empty, or APIResponses.
	API      string
	Glossary string // the user's own renderings, "source = target" lines
	Domain3D bool   // settings from before styles: the 3D style
	// ProfileID is the translation style; ProfilesDir holds the user's own.
	ProfileID, ProfilesDir string
	// OptimizeTimeline rebuilds an ordinary SRT's timeline by sense groups
	// before translating (see segment.go).
	OptimizeTimeline bool
	MaxRequests      int          // requests a run may send, retries included
	MaxOutputTokens  int          // the selected provider's advertised output cap; zero: unspecified
	Concurrency      int          // requests at once; 2 when zero
	CacheDir         string       // validated results, for resuming; none when empty
	Client           *http.Client `json:"-"`
	Observe          func(Event)  `json:"-"`
	Resume           *ResumeState `json:"-"`
}

// ResumeState keeps a frozen, validated segmentation and only completed texts.
// It contains no key. The input digest prevents applying progress to other cues.
type ResumeState struct {
	InputHash string            `json:"inputHash"`
	Units     subtitle.Document `json:"units"`
	Completed map[int]string    `json:"completed"`
	Context   json.RawMessage   `json:"context"`
}

type Report struct {
	Translated      int        `json:"translated"`
	Skipped         []string   `json:"skipped"`
	Warnings        []string   `json:"warnings"`
	Rules           []RuleFile `json:"rules"`
	Requests        int        `json:"requests"`
	InputTokens     int        `json:"inputTokens"`
	OutputTokens    int        `json:"outputTokens"`
	TimelineChanges int        `json:"timelineChanges"`
	// Rebuilt says the timeline was rebuilt: the result is checked against
	// the source's speech, not block by block.
	Rebuilt bool `json:"rebuilt"`
	Resumed int  `json:"resumed"` // subtitles taken from an earlier run
}

type Result struct {
	Source   subtitle.Document `json:"source"`   // the subtitles to translate, in the source language
	Document subtitle.Document `json:"document"` // the same subtitles, translated
	Report   Report            `json:"report"`
	Resume   *ResumeState      `json:"resume,omitempty"`
}

// Event is the real state of a run, for a live view. Units are sent once,
// when the subtitles to translate are known; Updates carry each one's
// translation as the model writes it.
type Event struct {
	Stage       string // "analyze", "segment" or "translate"
	Done, Total int
	Units       []subtitle.Cue
	Updates     []UnitText
	Thinking    string // the end of the model's latest reasoning, if it shows any
	Retry       string // why a request is sent again
	Requests    int
	Input       int          // tokens sent so far
	Output      int          // tokens received so far
	Resume      *ResumeState // sent once with the frozen units and context
}

type UnitText struct {
	Index int    // into Units
	Text  string // the translation so far
	Final bool   // checked and accepted
}

const (
	// version changes the cache key whenever the prompts change.
	version = "onesub-translate/3"
	// contextChars: sources up to this size go whole to every request;
	// longer ones are read once into a summary and working glossary.
	contextChars = 12000
	summaryChars = 40000 // source per summary request
	batchUnits   = 30
	batchChars   = 5000
	neighbors    = 5
)

const analyzeOperation = `# Operation: analyze
Read the complete source as the skill's workflow step 1 does: domain, register, speakers, recurring terms and names, UI strings, probable ASR mishearings. Choose one target-language rendering for each recurring concept: the working glossary. Do not translate the subtitles.
Return only a JSON object: {"summary": "a faithful account of the content, its register and speakers", "terms": ["source term → chosen rendering (short note)"]}.`

const mergeOperation = `# Operation: merge summaries
Merge the summaries and term lists of consecutive parts of one source into one, without dropping later parts, keeping one rendering per concept.
Return only a JSON object: {"summary": "...", "terms": ["..."]}.`

const translateOperation = `# Operation: translate
Translate each item of "units" into the target language: one finished subtitle per unit, the skill's single final rendering.
- "context" and "neighbors" are read-only, for meaning and consistent terms; neighbors carry the translations already done. Never translate or return them.
- Units are subtitles in time order. Where a sentence runs over several units, understand it as one thought, then give each unit the part of the translation that corresponds to its own source words, in source order (simultaneous-interpreting order). Never move content between units, merge, split or drop units.
- "lockedGlossary" lists the user's mandated renderings (LOCKED terms): use them exactly.
- Keep each unit's markup tags and line breaks with the text they belong to.
- "previousProblems", when present, says what was wrong in an earlier answer for these units: fix it.
Return only one JSON object whose keys are exactly the given unit IDs and whose values are their translations: {"<id>": "<translation>"}. No markdown, comments, alternatives or time codes.`

type overview struct {
	Summary string   `json:"summary"`
	Terms   []string `json:"terms"`
}

var overviewSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{"type": "string"},
		"terms":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"summary", "terms"},
	"additionalProperties": false,
}

type unitLine struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Translation string `json:"translation,omitempty"`
}

// translatePayload is ordered as sent: what every batch of a run shares
// comes first, so a service's prompt cache can reuse it.
type translatePayload struct {
	TargetLanguage   string     `json:"targetLanguage"`
	LockedGlossary   string     `json:"lockedGlossary,omitempty"`
	Context          any        `json:"context,omitempty"`
	Neighbors        []unitLine `json:"neighbors,omitempty"`
	PreviousProblems []string   `json:"previousProblems,omitempty"`
	Units            []unitLine `json:"units"`
}

type engine struct {
	config  Config
	svc     *service
	cache   cache
	lang    language
	profile Profile
	markup  bool
	key     string // what every cached result of this run depends on

	mu       sync.Mutex
	ruleSets map[string]ruleSet
	report   Report
	accepted map[int]string // translations accepted so far, by unit
	pending  map[int]UnitText
	flushed  time.Time
	stageNow Event
}

func (e *engine) concurrency() int {
	if e.config.Concurrency > 0 {
		return e.config.Concurrency
	}
	return 2
}

// system is a phase's system prompt: the adapter, the phase's skill files,
// the target profile, the style and the operation.
func (e *engine) system(phase, operation string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rules, ok := e.ruleSets[phase]
	if !ok {
		var err error
		if rules, err = readRules(e.config.SkillRoot, phasePaths(phase, e.lang, e.profile, e.markup)); err != nil {
			return "", err
		}
		e.ruleSets[phase] = rules
		for _, f := range rules.files {
			known := false
			for _, g := range e.report.Rules {
				known = known || g == f
			}
			if !known {
				e.report.Rules = append(e.report.Rules, f)
			}
		}
	}
	return systemPrompt(rules, e.profile, e.lang) + "\n" + operation, nil
}

func (e *engine) warn(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.report.Warnings = append(e.report.Warnings, text)
}

// keep caches a validated result. A cache that cannot be written costs
// only the resume; the run goes on and says so once.
func (e *engine) keep(key string, value any) {
	if err := e.cache.put(key, value); err != nil {
		e.mu.Lock()
		defer e.mu.Unlock()
		warning := locales.WarnCacheWrite(err.Error())
		for _, w := range e.report.Warnings {
			if w == warning {
				return
			}
		}
		e.report.Warnings = append(e.report.Warnings, warning)
	}
}

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

func endpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New(locales.ErrEndpointInvalid())
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New(locales.ErrEndpointParts())
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	local := host == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", errors.New(locales.ErrEndpointHttps())
	}
	// A pasted endpoint of a request, not the base URL, still works.
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/chat/completions")
	u.Path = strings.TrimSuffix(u.Path, "/responses")
	return u.String(), nil
}

// markupTags are the tags a subtitle carries: HTML-like ones, ASS override
// blocks and hard line breaks.
var markupTags = regexp.MustCompile(`<[^>\n]+>|\{\\[^}\n]*\}|\\[Nnh]`)

// Translate returns the translated subtitles; it does not change the
// project. Locked and protected subtitles keep their text. Every requested
// subtitle gets a translation or the run fails, saying how many did not:
// nothing is filled with the source. A run that fails or is cancelled
// resumes from its cache.
func Translate(ctx context.Context, config Config, document subtitle.Document) (result Result, err error) {
	result = Result{Document: document}
	base, err := endpoint(config.Endpoint)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(config.Model) == "" {
		return result, errors.New(locales.ErrTranslateModel())
	}
	if strings.TrimSpace(config.TargetLanguage) == "" {
		return result, errors.New(locales.ErrTranslateTarget())
	}
	profileID := config.ProfileID
	if profileID == "" && config.Domain3D {
		profileID = "3d"
	}
	profile, err := loadProfile(config.ProfilesDir, profileID)
	if err != nil {
		return result, err
	}
	lang, langFile, err := loadLanguage(config.SkillRoot, config.TargetLanguage)
	if err != nil {
		return result, err
	}
	e := &engine{config: config, cache: cache{config.CacheDir}, lang: lang, profile: profile, ruleSets: map[string]ruleSet{}, accepted: map[int]string{}}
	e.svc = newService(route(base, config.API), config.API, config.Model, config.Key, config.Client, config.MaxRequests)
	e.svc.maxOutputTokens = config.MaxOutputTokens
	e.svc.onRetry = e.retryNote
	e.report = Report{Skipped: []string{}, Warnings: []string{}, Rules: []RuleFile{langFile}}
	e.cache.prune()
	defer func() {
		e.flushUpdates()
		e.mu.Lock()
		result.Report = e.report
		if result.Resume != nil {
			result.Resume.Completed = make(map[int]string, len(e.accepted))
			for i, text := range e.accepted {
				result.Resume.Completed[i] = text
			}
		}
		e.mu.Unlock()
		result.Report.Requests, result.Report.InputTokens, result.Report.OutputTokens = e.svc.usage()
	}()

	doc := document
	doc.Cues = append([]subtitle.Cue{}, document.Cues...)
	writable, size := false, 0
	for i := range doc.Cues {
		cue := &doc.Cues[i]
		if cue.ReadOnly || cue.TextLocked {
			continue
		}
		writable = true
		cue.Text = subtitle.SourceText(*cue)
		size += len(cue.Text)
		e.markup = e.markup || markupTags.MatchString(cue.Text)
	}
	if !writable {
		return result, errors.New(locales.ErrNothingToTranslate())
	}
	e.key = cacheKey(version, base, config.API, config.Model, config.TargetLanguage, profile.SHA256, profile.References, config.Glossary, langFile.SHA256, e.markup)
	if config.Resume != nil && config.Resume.InputHash != cacheKey(document) {
		return result, errors.New(locales.TranslationResumeStale())
	}

	// 1. The whole source, read before anything is decided.
	var summary *overview
	if config.Resume == nil && size > contextChars {
		e.stage("analyze", 0, 0)
		if summary, err = e.analyze(ctx, doc); err != nil {
			return result, err
		}
	}
	// 2. The timeline, by sense groups or as it is.
	var ranges []subtitle.SourceRange
	if config.Resume != nil {
		if err = validateResume(doc, config.Resume); err != nil {
			return result, err
		}
		for _, cue := range config.Resume.Units.Cues {
			if cue.SourceRange != nil {
				ranges = append(ranges, *cue.SourceRange)
			} else {
				one, rangeErr := sourceRanges(subtitle.Document{Format: doc.Format, Cues: []subtitle.Cue{cue}})
				if rangeErr != nil {
					return result, rangeErr
				}
				ranges = append(ranges, one...)
			}
		}
	} else if config.OptimizeTimeline && doc.Format == "srt" {
		var read any
		if summary != nil {
			read = summary
		}
		if ranges, err = e.rebuild(ctx, doc, read); err != nil {
			return result, err
		}
	} else if ranges, err = sourceRanges(doc); err != nil {
		return result, err
	}
	units, warnings, err := subtitle.Resegment(doc, ranges)
	if err != nil {
		return result, err
	}
	e.report.Warnings = append(e.report.Warnings, warnings...)
	for _, cue := range units.Cues {
		if cue.SourceRange != nil && cue.SourceRange.Reason != "" {
			e.report.TimelineChanges++
		}
	}
	e.report.Rebuilt = e.report.TimelineChanges > 0
	result.Source = units
	result.Document = units
	result.Document.Cues = append([]subtitle.Cue{}, units.Cues...)

	// 3. The translation, in batches with their context.
	todo := []int{}
	for i, cue := range units.Cues {
		if cue.ReadOnly || cue.TextLocked {
			e.report.Skipped = append(e.report.Skipped, cue.ID)
		} else {
			todo = append(todo, i)
		}
	}
	var shared any = summary
	if config.Resume != nil && len(config.Resume.Context) > 0 {
		if err = json.Unmarshal(config.Resume.Context, &shared); err != nil {
			return result, err
		}
	} else if summary == nil {
		lines := []unitLine{}
		for _, i := range todo {
			lines = append(lines, unitLine{ID: unitID(i), Text: units.Cues[i].Text})
		}
		shared = lines
	}
	result.Resume = &ResumeState{InputHash: cacheKey(document), Units: units, Context: json.RawMessage(marshal(shared))}
	e.mu.Lock()
	e.stageNow = Event{Stage: "translate", Total: len(todo)}
	progress := *result.Resume
	e.emit(Event{Units: append([]subtitle.Cue{}, units.Cues...), Resume: &progress})
	e.mu.Unlock()
	if config.Resume != nil {
		for i, text := range config.Resume.Completed {
			e.update(i, text, true, "")
			e.report.Resumed++
		}
	}
	failed, err := e.translateAll(ctx, units.Cues, todo, shared)
	// Assemble accepted units even when a later request fails or is cancelled.
	// Incomplete units stay visibly untranslated and cannot be exported as done.
	for _, i := range todo {
		text, accepted := e.accepted[i]
		if !accepted {
			continue
		}
		cue := &result.Document.Cues[i]
		cue.TranslationSource = subtitle.SourceText(*cue)
		cue.Translated = true
		cue.Text = polish(text, lang)
		if missing := missingNumbers(cue.TranslationSource, cue.Text); len(missing) > 0 {
			e.report.Warnings = append(e.report.Warnings, locales.WarnNumbers(i+1, strings.Join(missing, locales.ListSeparator())))
		}
		e.report.Translated++
	}
	if err != nil {
		return result, err
	}
	if len(failed) > 0 {
		return result, errors.New(locales.ErrTranslateIncomplete(len(failed)))
	}
	return result, nil
}

// sourceRanges keeps every source subtitle as it is.
func sourceRanges(doc subtitle.Document) ([]subtitle.SourceRange, error) {
	atoms, err := subtitle.TimelineAtoms(doc)
	if err != nil {
		return nil, err
	}
	ranges := []subtitle.SourceRange{}
	for i := 0; i < len(atoms); {
		j := i + 1
		for j < len(atoms) && atoms[j].CueID == atoms[i].CueID {
			j++
		}
		ranges = append(ranges, subtitle.SourceRange{FirstAtom: atoms[i].ID, LastAtom: atoms[j-1].ID})
		i = j
	}
	return ranges, nil
}

// unitID is how a model sees a subtitle: its number in the file.
func unitID(index int) string { return strconv.Itoa(index + 1) }

// analyze reads a long source into one summary and working glossary: in
// parts, whose summaries are then merged, so the end of a film counts as
// much as its beginning.
func (e *engine) analyze(ctx context.Context, doc subtitle.Document) (*overview, error) {
	parts := []overview{}
	var window strings.Builder
	read := func() error {
		if window.Len() == 0 {
			return nil
		}
		var o overview
		input := "Target language: " + targetName(e.lang, e.config.TargetLanguage) + "\n<source>\n" + window.String() + "</source>"
		if err := e.structured(ctx, "analyze", analyzeOperation, input, &o); err != nil {
			return err
		}
		parts = append(parts, o)
		window.Reset()
		return nil
	}
	for _, cue := range doc.Cues {
		if cue.ReadOnly {
			continue // drawings and other protected non-dialogue are not read
		}
		line := strings.Join(strings.Fields(cue.Text), " ") + "\n"
		if window.Len()+len(line) > summaryChars {
			if err := read(); err != nil {
				return nil, err
			}
		}
		window.WriteString(line)
	}
	if err := read(); err != nil {
		return nil, err
	}
	for len(parts) > 1 {
		merged := []overview{}
		for start := 0; start < len(parts); start += 4 {
			var o overview
			input := "Target language: " + targetName(e.lang, e.config.TargetLanguage) + "\n<summaries>\n" + marshal(parts[start:min(start+4, len(parts))]) + "\n</summaries>"
			if err := e.structured(ctx, "analyze", mergeOperation, input, &o); err != nil {
				return nil, err
			}
			merged = append(merged, o)
		}
		parts = merged
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return &parts[0], nil
}

// structured asks for one overview object, from the cache when it has it.
func (e *engine) structured(ctx context.Context, phase, operation, input string, out *overview) error {
	key := cacheKey(e.key, phase, operation, input)
	if e.cache.get(key, out) {
		return nil
	}
	system, err := e.system(phase, operation)
	if err != nil {
		return err
	}
	r := request{system: system, messages: []message{{"user", input}}, schema: overviewSchema}
	for attempt := 0; ; attempt++ {
		got, err := e.svc.call(ctx, r, func(_, thinking string) {
			if thinking != "" {
				e.update(-1, "", false, thinking)
			}
		})
		if err != nil {
			return err
		}
		text := stripThinking(got.text)
		for _, candidate := range append(fenceBlock.FindAllString(text, -1), firstObject(text), text) {
			candidate = strings.Trim(strings.TrimSpace(candidate), "`")
			candidate = strings.TrimPrefix(candidate, "json")
			if json.Unmarshal([]byte(strings.TrimSpace(candidate)), out) == nil && out.Summary != "" {
				e.keep(key, out)
				return nil
			}
		}
		if attempt == 1 {
			return errors.New(locales.ErrSummaryInvalid())
		}
		r.messages = append(r.messages, message{"assistant", got.text}, message{"user", `Return only the JSON object {"summary": "...", "terms": ["..."]}.`})
	}
}

func marshal(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// translateAll translates the units in todo, in batches sent concurrently.
// It returns the units that stayed untranslated after every correction.
func (e *engine) translateAll(ctx context.Context, units []subtitle.Cue, todo []int, shared any) ([]int, error) {
	batches := [][]int{}
	for start := 0; start < len(todo); {
		end, chars := start, 0
		for end < len(todo) && end-start < batchUnits {
			n := len(units[todo[end]].Text)
			if end > start && chars+n > batchChars {
				break
			}
			chars += n
			end++
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
	for range e.concurrency() {
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
				valid := text != "" && sameMarkup(units[i].Text, text) && (round >= 2 || !untranslated(units[i].Text, text, e.lang))
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

func validateResume(doc subtitle.Document, resume *ResumeState) error {
	candidate := resume.Units
	candidate.Cues = append([]subtitle.Cue{}, candidate.Cues...)
	for i := range candidate.Cues {
		cue := &candidate.Cues[i]
		if !cue.ReadOnly && !cue.TextLocked {
			cue.TranslationSource = subtitle.SourceText(*cue)
			cue.Translated = true
		}
	}
	if err := subtitle.ValidateTranslation(doc, candidate); err != nil {
		return err
	}
	for i, text := range resume.Completed {
		if i < 0 || i >= len(resume.Units.Cues) || strings.TrimSpace(text) == "" {
			return errors.New(locales.TranslationResumeStale())
		}
		cue := resume.Units.Cues[i]
		if cue.ReadOnly || cue.TextLocked || !sameMarkup(cue.Text, text) {
			return errors.New(locales.TranslationResumeStale())
		}
	}
	return nil
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

func sameMarkup(source, translation string) bool {
	a, b := markupTags.FindAllString(source, -1), markupTags.FindAllString(translation, -1)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// untranslated catches an answer that copied its source, the failure
// SmartSub's untranslated check exists for: the same letters, or none of
// the target's script at all where the source has none either. A name or a
// code alone is short and passes.
func untranslated(source, translation string, l language) bool {
	letters := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	a := letters(source)
	if len([]rune(a)) < 8 {
		return false
	}
	if a == letters(translation) {
		return true
	}
	script := targetScript(l)
	return script != nil && !strings.ContainsFunc(source, script) && !strings.ContainsFunc(translation, script)
}

// targetScript is the script a translation into l is written in, where
// it differs from Latin: Han for Chinese, kana or Han for Japanese, Hangul
// for Korean.
func targetScript(l language) func(rune) bool {
	switch {
	case l.chinese():
		return func(r rune) bool { return unicode.Is(unicode.Han, r) }
	case l.Code == "ja":
		return func(r rune) bool {
			return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
		}
	case l.Code == "ko":
		return func(r rune) bool { return unicode.Is(unicode.Hangul, r) }
	}
	return nil
}

// isCJK is the skill checker's CJK_RE: ideographs and kana.
func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0xF900 && r <= 0xFAFF) || (r >= 0x3040 && r <= 0x30FF)
}

var numbers = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// missingNumbers lists the numbers of the source the translation lacks:
// dropped payload. Numbers the translation adds, as a spelled-out "three"
// written 3, are what the skill asks for.
func missingNumbers(source, translation string) []string {
	count := map[string]int{}
	for _, n := range numbers.FindAllString(translation, -1) {
		count[strings.ReplaceAll(n, ",", "")]++
	}
	missing := []string{}
	for _, n := range numbers.FindAllString(source, -1) {
		key := strings.ReplaceAll(n, ",", "")
		if count[key] > 0 {
			count[key]--
		} else {
			missing = append(missing, n)
		}
	}
	return missing
}

// stampMs writes a time as 00:01:02.
func stampMs(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

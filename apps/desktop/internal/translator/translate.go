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
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

const (
	// version changes the cache key whenever the prompts change.
	version = "onesub-translate/5"
	// contextChars: sources up to this size go whole to every request;
	// longer ones are read once into a summary and working glossary.
	contextChars = 12000
	summaryChars = 40000 // source per summary request
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
- "durationMs" is the actual display duration. "readingBudget" gives soft targets in the target profile's counting units, with a separate per-line limit. Keep facts, names, numbers, negation, conditions, terminology and formatting even when they cannot fit. Prefer concise faithful wording; never truncate, invent time, move information or omit a unit to meet a budget. Parenthetical original-language annotations follow the skill's counting exemptions. Speaker metadata is context only, never text to translate or prefix.
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
	ID            string                      `json:"id"`
	Text          string                      `json:"text"`
	Translation   string                      `json:"translation,omitempty"`
	DurationMs    int64                       `json:"durationMs,omitempty"`
	ReadingBudget *ReadingBudget              `json:"readingBudget,omitempty"`
	Speaker       *subtitle.SpeakerAssignment `json:"speaker,omitempty"`
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
	config        Config
	svc           *service
	cache         cache
	lang          language
	profile       Profile
	markup        bool
	key           string // what every cached result of this run depends on
	budgetVersion int

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
	return DefaultConcurrency
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
	e.budgetVersion = readingBudgetVersion
	if config.Resume != nil {
		// Older interrupted jobs keep their already accepted work and old mode.
		e.budgetVersion = config.Resume.BudgetVersion
		if e.budgetVersion != 0 && (e.budgetVersion != readingBudgetVersion || config.Resume.BudgetRules != cacheKey(lang)) {
			return result, errors.New(locales.TranslationResumeStale())
		}
	}
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
		writable = writable || !config.OnlyMissing || cue.MissingTranslation()
		if !config.OnlyMissing || cue.MissingTranslation() {
			cue.Text = subtitle.SourceText(*cue)
		}
		size += len(cue.Text)
		e.markup = e.markup || markupTags.MatchString(cue.Text)
	}
	if !writable {
		return result, errors.New(locales.ErrNothingToTranslate())
	}
	e.key = cacheKey(version, base, config.API, config.Model, config.TargetLanguage, profile.SHA256, profile.References, config.Glossary, langFile.SHA256, e.markup)
	if config.CacheScope != "" {
		// Keep legacy interrupted tasks readable; new task scopes isolate all
		// cached phases, including analysis and timeline segmentation.
		e.key = cacheKey(e.key, config.CacheScope)
	}
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
	} else if config.OptimizeTimeline && !config.OnlyMissing && doc.Format == "srt" {
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
		if cue.ReadOnly || cue.TextLocked || config.OnlyMissing && !cue.MissingTranslation() {
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
	result.Resume = &ResumeState{InputHash: cacheKey(document), Units: units, Context: json.RawMessage(marshal(shared)), BudgetVersion: e.budgetVersion, BudgetRules: cacheKey(lang)}
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
		outputLanguage := lang
		if config.Punctuation != nil {
			// Explicit local preferences supersede the profile's mechanical
			// trailing trim, not its prompt or any model-generated wording.
			outputLanguage.FinalPunctuation = ""
		}
		cue.Text = polish(text, outputLanguage)
		if config.Punctuation != nil {
			cue.Text = config.Punctuation.Apply(cue.Text)
		}
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

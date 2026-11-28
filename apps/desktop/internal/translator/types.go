package translator

import (
	"encoding/json"
	"net/http"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
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
	Punctuation      *subtitle.PunctuationOptions // local output only; never sent to the service
	OptimizeTimeline bool
	OnlyMissing      bool         // explicit continuation of an accepted partial track
	MaxRequests      int          // requests a run may send, retries included
	MaxOutputTokens  int          // the selected provider's advertised output cap; zero: unspecified
	Concurrency      int          // requests at once; 2 when zero
	BatchSize        int          // subtitles per translation request; 20 when zero
	CacheDir         string       // validated results, for resuming; none when empty
	CacheScope       string       // persisted task identity; fresh tasks must not reuse old answers
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

// Defaults apply to new runs and checkpoints created before these settings existed.
const DefaultConcurrency = 2
const DefaultBatchSize = 20

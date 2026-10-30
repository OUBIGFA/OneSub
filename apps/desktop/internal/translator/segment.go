package translator

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Sense groups (意群). The skill repairs seams before it translates: an
// orphan tail, a stranded connector or a split term joins the subtitle it
// belongs to, and a long sentence divides where a natural phrase boundary
// and timing evidence allow. As VideoCaptioner's LLM split does, the model
// re-breaks the source text into subtitle lines, and OneSub maps the lines
// back onto the source atoms. So every word and every time of the result
// is the source's: a line the model reworded or dropped cannot change the
// text, only fail to move a boundary.

const (
	// windowMs is the source duration one segmentation request covers: a
	// 30-minute video is one batch, an hour at most two.
	windowMs      = int64(30 * 60 * 1000)
	minSimilarity = 0.96 // VideoCaptioner's bar for "only line breaks changed"
	// maxJoinGap is the longest pause a joined subtitle may span, in ms. A
	// longer one is a real pause, cut as VideoCaptioner's MAX_GAP cuts it:
	// the subtitle would otherwise stand through the silence.
	maxJoinGap = 1500
	// maxJoined is the longest subtitle joining may make, in ms: past it,
	// the join is undone at its largest pause.
	maxJoined = 10000
	// minPiece is the shortest piece of a divided source subtitle, in ms,
	// the skill's checker's min_split_piece; a shorter one rejoins.
	minPiece = 1000
)

const segmentOperation = `# Operation: segment
Repair the seams of the source subtitles before they are translated (the skill's repair step; ordinary SRT mode). Do not translate.
Input: the user message names the target language, says whether word timing is available, and lists the source blocks in <source>, one per line, each after its time span in seconds as [start→end].
Output: the same source text broken into subtitles, one subtitle per line, without the time spans, and nothing else: no numbering, no comments.
- Keep every word, number and punctuation mark exactly as given and in the same order; change only where lines break.
- Join a line with the next only to repair a specific seam defect of references/segmentation.md: an orphan completion, a stranded connector, a dependent opening, a split term or UI label, a trailing modifier, a filler-only flash. Topic continuity, fast speech or a shared subject is not a reason. Keep natural multi-block sentences and every useful boundary.
- With word timing: yes, you may also break a long line at a natural phrase or clause boundary, so that each subtitle carries one sense group that its translation can be read in, within the target language profile; never inside a name, term, number–unit pair, or between a verb and its required complement. With word timing: no, never break a line.`

// timePrefix is a time span a model copied into its lines.
var timePrefix = regexp.MustCompile(`^\[[0-9.:]+\s*(?:→|->|-|–)\s*[0-9.:]+\]\s*`)

// cueSpan is one source cue's atoms, first to last.
type cueSpan struct{ first, last int }

// rebuild returns the sense-group ranges of doc's atoms. The model decides
// boundaries only inside runs of ordinary cues; protected cues keep theirs.
func (e *engine) rebuild(ctx context.Context, doc subtitle.Document, summary any) ([]subtitle.SourceRange, error) {
	atoms, err := subtitle.TimelineAtoms(doc)
	if err != nil {
		return nil, err
	}
	spans := []cueSpan{}
	for i := 0; i < len(atoms); {
		j := i
		for j+1 < len(atoms) && atoms[j+1].CueID == atoms[i].CueID {
			j++
		}
		spans = append(spans, cueSpan{i, j})
		i = j + 1
	}
	// starts marks the atoms that begin a subtitle; the source's own
	// boundaries until a window's answer replaces them.
	starts := make([]bool, len(atoms))
	for _, s := range spans {
		starts[s.first] = true
	}
	windows := segmentWindows(atoms, spans)
	e.stage("segment", 0, len(windows))
	var (
		mu       sync.Mutex
		done     int
		firstErr error
	)
	work := make(chan []cueSpan)
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for range e.concurrency() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for window := range work {
				cuts, err := e.segmentWindow(ctx, atoms, window, summary)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
						cancel()
					}
				} else {
					for i := window[0].first + 1; i <= window[len(window)-1].last; i++ {
						starts[i] = false
					}
					for _, cut := range cuts {
						starts[cut] = true
					}
					done++
					e.stage("segment", done, len(windows))
				}
				mu.Unlock()
			}
		}()
	}
	for _, window := range windows {
		select {
		case work <- window:
		case <-ctx.Done():
		}
	}
	close(work)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	guardSenseGroups(atoms, spans, starts)
	ranges := []subtitle.SourceRange{}
	cueOf := map[string]cueSpan{}
	for _, s := range spans {
		cueOf[atoms[s.first].CueID] = s
	}
	for i := 0; i < len(atoms); {
		j := i + 1
		for j < len(atoms) && !starts[j] {
			j++
		}
		first, last := atoms[i], atoms[j-1]
		r := subtitle.SourceRange{FirstAtom: first.ID, LastAtom: last.ID}
		switch {
		case first.CueID != last.CueID:
			r.Reason = locales.ReasonJoined()
		case cueOf[first.CueID].first != i || cueOf[first.CueID].last != j-1:
			r.Reason = locales.ReasonDivided()
		}
		ranges = append(ranges, r)
		i = j
	}
	return ranges, nil
}

// segmentWindows groups runs of ordinary cues into requests covering
// about windowMs of the timeline each: a 30-minute video is one batch, an
// hour at most two. A run is cut into round(duration/windowMs) windows
// whose boundaries land at the cue boundary nearest its target time — the
// largest pause when several are equally near, as VideoCaptioner chooses
// its split points.
func segmentWindows(atoms []subtitle.Atom, spans []cueSpan) [][]cueSpan {
	windows := [][]cueSpan{}
	run := []cueSpan{}
	split := func() {
		if len(run) == 0 {
			return
		}
		start, end := atoms[run[0].first].StartMs, atoms[run[len(run)-1].last].EndMs
		n := int((end - start + windowMs/2) / windowMs)
		if n < 1 {
			n = 1
		}
		// Whole cues at least: never more windows than cues.
		if n > len(run) {
			n = len(run)
		}
		prev := 0
		for i := 1; i < n; i++ {
			target := start + (end-start)*int64(i)/int64(n)
			best, bestDist, bestGap := -1, int64(0), int64(0)
			for k := prev; k < len(run)-1; k++ {
				dist := atoms[run[k+1].first].StartMs - target
				if dist < 0 {
					dist = -dist
				}
				gap := atoms[run[k+1].first].StartMs - atoms[run[k].last].EndMs
				if best < 0 || dist < bestDist || (dist == bestDist && gap > bestGap) {
					best, bestDist, bestGap = k, dist, gap
				}
			}
			if best < 0 {
				break
			}
			windows = append(windows, run[prev:best+1])
			prev = best + 1
		}
		windows = append(windows, run[prev:])
		run = nil
	}
	for _, s := range spans {
		if atoms[s.first].Protected {
			split()
			continue
		}
		run = append(run, s)
	}
	split()
	return windows
}

// segmentWindow asks for one window's lines and returns the atoms that
// start a subtitle in it, its first excepted. An answer that changed the
// words gets one correction request, as VideoCaptioner's split loop does;
// a second leaves the window's source boundaries in place, with a warning.
func (e *engine) segmentWindow(ctx context.Context, atoms []subtitle.Atom, window []cueSpan, summary any) ([]int, error) {
	first, last := window[0].first, window[len(window)-1].last
	texts := make([]string, 0, last-first+1)
	words := false
	var source strings.Builder
	for _, s := range window {
		words = words || s.last > s.first
		var line strings.Builder
		for i := s.first; i <= s.last; i++ {
			line.WriteString(atoms[i].Text)
		}
		fmt.Fprintf(&source, "[%.2f→%.2f] %s\n", float64(atoms[s.first].StartMs)/1000, float64(atoms[s.last].EndMs)/1000, strings.Join(strings.Fields(line.String()), " "))
	}
	for i := first; i <= last; i++ {
		texts = append(texts, atoms[i].Text)
	}
	timing := "no"
	if words {
		timing = "yes"
	}
	input := "Target language: " + targetName(e.lang, e.config.TargetLanguage) + "\nWord timing: " + timing + "\n"
	if summary != nil {
		input += "Read-only summary of the whole source: " + marshal(summary) + "\n"
	}
	input += "<source>\n" + source.String() + "</source>"
	key := cacheKey(e.key, "segment", input)
	var lines []string
	a := alignment{}
	if e.cache.get(key, &lines) {
		a = alignLines(texts, lines)
	} else {
		system, err := e.system("segment", segmentOperation)
		if err != nil {
			return nil, err
		}
		r := request{system: system, messages: []message{{"user", input}}}
		for attempt := 0; ; attempt++ {
			got, err := e.svc.call(ctx, r, nil)
			if err != nil {
				return nil, err
			}
			lines = answerLines(got.text)
			a = alignLines(texts, lines)
			if a.similarity >= minSimilarity && !got.truncated {
				e.keep(key, lines)
				break
			}
			if attempt == 1 {
				// The source's own boundaries, unchanged.
				e.warn(locales.WarnSegmentKept(stampMs(atoms[first].StartMs), stampMs(atoms[last].EndMs)))
				cuts := []int{}
				for _, s := range window[1:] {
					cuts = append(cuts, s.first)
				}
				return cuts, nil
			}
			r.messages = append(r.messages, message{"assistant", got.text}, message{"user", fmt.Sprintf("Your lines changed the source words (%.0f%% match) or stopped early. Keep every word, number and punctuation mark exactly, in order; change only where lines break. Return the complete text again, one subtitle per line.", a.similarity*100)})
		}
	}
	cuts := []int{}
	for _, cut := range a.cuts {
		cuts = append(cuts, first+cut)
	}
	return cuts, nil
}

// answerLines reads a plain answer: one subtitle per line, without the
// reasoning, fences or time spans a model may add.
func answerLines(text string) []string {
	text = stripThinking(text)
	if m := fenceBlock.FindStringSubmatch(text); m != nil {
		text = m[1]
	}
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "<br>", "\n"), "\n") {
		if line = strings.TrimSpace(timePrefix.ReplaceAllString(strings.TrimSpace(line), "")); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// guardSenseGroups keeps what a model may not decide about time. Protected
// atoms stand alone. Overlapping cues, and cues apart by more than
// maxJoinGap, are never joined. A joined subtitle longer than maxJoined is
// divided at its largest pause until it is not, or is one cue again. A
// piece divided out of one cue shorter than minPiece rejoins its neighbor
// in that cue.
func guardSenseGroups(atoms []subtitle.Atom, spans []cueSpan, starts []bool) {
	for i, atom := range atoms {
		if atom.Protected {
			starts[i] = true
			if i+1 < len(atoms) {
				starts[i+1] = true
			}
		}
		if i > 0 && atom.CueID != atoms[i-1].CueID {
			if gap := atom.StartMs - atoms[i-1].EndMs; gap < 0 || gap > maxJoinGap {
				starts[i] = true
			}
		}
	}
	units := func() [][2]int {
		out := [][2]int{}
		for i := 0; i < len(atoms); {
			j := i + 1
			for j < len(atoms) && !starts[j] {
				j++
			}
			out = append(out, [2]int{i, j - 1})
			i = j
		}
		return out
	}
	for changed := true; changed; {
		changed = false
		for _, u := range units() {
			if atoms[u[0]].CueID == atoms[u[1]].CueID || atoms[u[1]].EndMs-atoms[u[0]].StartMs <= maxJoined {
				continue
			}
			cut, widest := -1, int64(-1)
			for i := u[0] + 1; i <= u[1]; i++ {
				if atoms[i].CueID != atoms[i-1].CueID {
					if gap := atoms[i].StartMs - atoms[i-1].EndMs; gap > widest {
						cut, widest = i, gap
					}
				}
			}
			starts[cut] = true
			changed = true
		}
	}
	cueOf := make([]cueSpan, len(atoms))
	for _, s := range spans {
		for i := s.first; i <= s.last; i++ {
			cueOf[i] = s
		}
	}
	for changed := true; changed; {
		changed = false
		list := units()
		for _, u := range list {
			s := cueOf[u[0]]
			inside := cueOf[u[1]] == s && (u[0] != s.first || u[1] != s.last)
			if !inside || atoms[u[1]].EndMs-atoms[u[0]].StartMs >= minPiece {
				continue
			}
			if u[0] != s.first {
				starts[u[0]] = false // into the piece before it
			} else {
				starts[u[1]+1] = false // into the piece after it, in the same cue
			}
			changed = true
			break
		}
	}
}

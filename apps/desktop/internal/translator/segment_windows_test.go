package translator

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Temporal segmentation windows must not omit or repeat a cue when the
// requests are larger than the old count/character windows.
func TestTemporalSegmentationWindowsCoverEveryCue(t *testing.T) {
	for _, minutes := range []int{1, 30, 60, 90} {
		t.Run(fmt.Sprint(minutes), func(t *testing.T) {
			var atoms []subtitle.Atom
			var spans []cueSpan
			for i := range minutes * 12 {
				atoms = append(atoms, subtitle.Atom{StartMs: int64(i * 5000), EndMs: int64((i + 1) * 5000), Text: "Source subtitle"})
				spans = append(spans, cueSpan{i, i})
			}
			windows := segmentWindows(atoms, spans)
			want := max(1, minutes/30)
			if len(windows) != want {
				t.Fatal("wrong number of temporal windows", len(windows), want)
			}
			seen := 0
			for _, window := range windows {
				if len(window) == 0 {
					t.Fatal("empty window")
				}
				for _, span := range window {
					if span.first != seen || span.last != seen {
						t.Fatal("cue omitted or repeated", span, seen)
					}
					seen++
				}
			}
			if seen != len(atoms) {
				t.Fatal("tail missing")
			}
		})
	}
}

func TestTemporalSegmentationDoesNotIncludeProtectedCues(t *testing.T) {
	atoms := []subtitle.Atom{{StartMs: 0, EndMs: 1000}, {StartMs: 1000, EndMs: 2000, Protected: true}, {StartMs: 2000, EndMs: 3000}}
	windows := segmentWindows(atoms, []cueSpan{{0, 0}, {1, 1}, {2, 2}})
	if len(windows) != 2 || len(windows[0]) != 1 || len(windows[1]) != 1 || windows[0][0].first != 0 || windows[1][0].first != 2 {
		t.Fatal(windows)
	}
}

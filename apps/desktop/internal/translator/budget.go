package translator

import "github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"

const readingBudgetVersion = 1

// ReadingBudget is guidance, never permission to remove information or change
// timing. Units use the target profile's reading convention, not UTF-8 bytes.
type ReadingBudget struct {
	Units        float64 `json:"units"`
	MaxLineUnits float64 `json:"maxLineUnits"`
	Counting     string  `json:"counting"`
}

func (e *engine) translationUnit(index int, cue subtitle.Cue) unitLine {
	line := unitLine{ID: unitID(index), Text: cue.Text, Speaker: cue.Speaker}
	if e.budgetVersion == 0 {
		return line
	}
	rate, width := e.lang.MaxCPS, e.lang.MaxWidth
	if e.lang.Counting == "raw" {
		rate = min(rate, 17)
		width = min(width, 64)
	}
	line.DurationMs = cue.EndMs - cue.StartMs
	line.ReadingBudget = &ReadingBudget{Units: float64(line.DurationMs) * rate / 1000, MaxLineUnits: width, Counting: e.lang.Counting}
	return line
}

// Cache all request-shaping per-line inputs together, including its clock.
func (e *engine) unitCacheKey(contextKey string, index int, cue subtitle.Cue) string {
	return cacheKey(contextKey, e.budgetVersion, e.translationUnit(index, cue))
}

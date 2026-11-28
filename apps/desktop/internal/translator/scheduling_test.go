package translator

import (
	"context"
	"testing"
)

// A completed resume can leave no batches. The configured request ceiling
// must not turn that no-op into thousands of goroutine allocations.
func TestEmptyTranslationDoesNotAllocateAWorkerPool(t *testing.T) {
	e := &engine{config: Config{Concurrency: 10000}}
	allocations := testing.AllocsPerRun(3, func() {
		failed, err := e.translateAll(context.Background(), nil, nil, nil)
		if err != nil || len(failed) != 0 {
			t.Fatal(failed, err)
		}
	})
	if allocations > 20 {
		t.Fatalf("empty work allocated %.0f objects", allocations)
	}
}

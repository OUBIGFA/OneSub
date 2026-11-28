package jobs

import (
	"bytes"
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// BoundedBuffer captures one control stream. Unlike the diagnostic tail,
// protocol output must fail on overflow rather than discard earlier bytes.
type BoundedBuffer struct {
	buffer   bytes.Buffer
	Limit    int
	Overflow error // optional domain-specific error
}

// Do not embed bytes.Buffer: its promoted ReadFrom lets io.Copy bypass Write,
// which is exactly how os/exec drains a child's stdout and stderr.
func (b *BoundedBuffer) Bytes() []byte { return b.buffer.Bytes() }
func (b *BoundedBuffer) Len() int      { return b.buffer.Len() }

func (b *BoundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.Limit-b.Len() {
		if b.Overflow != nil {
			return 0, b.Overflow
		}
		return 0, errors.New(locales.ErrWorkerOutput())
	}
	return b.buffer.Write(p)
}

package translator

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// A valid answer followed by a broken body is still an interrupted request.
type brokenBody struct{ io.Reader }

func (r brokenBody) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return n, err
}

func TestResponseRejectsOversizedSingleLineAndInterruptedJSON(t *testing.T) {
	s := &service{api: APIChatCompletions}
	for _, body := range []string{
		`{"choices":[{"message":{"content":"` + strings.Repeat("x", maxAnswer) + `"},"finish_reason":"stop"}]}`,
		"data: " + strings.Repeat(" ", maxAnswer) + "\n",
	} {
		if _, err := s.read(strings.NewReader(body), func() {}, nil); err == nil {
			t.Fatal("oversized response accepted")
		}
	}
	body := "{\n\"choices\":[{\"message\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}"
	if _, err := s.read(brokenBody{strings.NewReader(body)}, func() {}, nil); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("body read failure was lost: %v", err)
	}
	got, err := s.read(strings.NewReader(body), func() {}, nil)
	if err != nil || got.text != "done" {
		t.Fatalf("complete answer changed: %+v, %v", got, err)
	}
}

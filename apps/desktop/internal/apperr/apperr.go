// Package apperr gives user-facing errors a message in the app's language
// while keeping the wrapped cause, so errors.Is and errors.As still see
// cancellation and file-system errors underneath.
package apperr

type wrapped struct {
	message string
	cause   error
}

func (w *wrapped) Error() string { return w.message }
func (w *wrapped) Unwrap() error { return w.cause }

// Wrap returns cause with message, a complete sentence already said in the
// app's language. The message includes the cause's text where it helps.
func Wrap(message string, cause error) error {
	return &wrapped{message: message, cause: cause}
}

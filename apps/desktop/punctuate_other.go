//go:build !windows

package main

import "errors"

// runPunctuate needs the Windows build of sherpa-onnx's C API.
func runPunctuate([]string) error {
	return errors.New("punctuation is available on Windows only")
}

//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// punctuationConfig is SherpaOnnxOfflinePunctuationConfig of the pinned
// sherpa-onnx v2.0.1 C API (c-api.h): two pointers around two int32s.
type punctuationConfig struct {
	CtTransformer *byte
	NumThreads    int32
	Debug         int32
	Provider      *byte
}

// runPunctuate is "OneSub.exe punctuate RUNTIME MODELDIR", a child process
// that adds punctuation with the sherpa-onnx C API. Its command-line tool
// reads text in the ANSI code page, which loses Chinese on most systems,
// so texts come in as JSON strings, one a line, on stdin and go out the
// same way on stdout. The model is opened by a relative name in its own
// folder, so no non-ASCII path reaches the library either.
func runPunctuate(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: punctuate RUNTIME MODELDIR")
	}
	// Windows 11 has an older onnxruntime.dll of its own in System32. Load
	// the bundled one by path first, so the C API binds to the version it
	// was built with.
	var dll windows.Handle
	var err error
	for _, name := range []string{"onnxruntime.dll", "sherpa-onnx-c-api.dll"} {
		if dll, err = windows.LoadLibraryEx(filepath.Join(args[0], name), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH); err != nil {
			return err
		}
	}
	procs := map[string]uintptr{}
	for _, name := range []string{"SherpaOnnxCreateOfflinePunctuation", "SherpaOnnxDestroyOfflinePunctuation", "SherpaOfflinePunctuationAddPunct", "SherpaOfflinePunctuationFreeText"} {
		if procs[name], err = windows.GetProcAddress(dll, name); err != nil {
			return err
		}
	}
	if err = os.Chdir(args[1]); err != nil {
		return err
	}
	model, provider := []byte("punct.onnx\x00"), []byte("cpu\x00")
	config := punctuationConfig{CtTransformer: &model[0], NumThreads: int32(min(4, max(1, runtime.NumCPU()/2))), Provider: &provider[0]}
	handle, _, _ := syscall.SyscallN(procs["SherpaOnnxCreateOfflinePunctuation"], uintptr(unsafe.Pointer(&config)))
	runtime.KeepAlive(model)
	runtime.KeepAlive(provider)
	if handle == 0 {
		return errors.New("punctuation model failed to load")
	}
	defer syscall.SyscallN(procs["SherpaOnnxDestroyOfflinePunctuation"], handle)
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 64*1024), 4*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()
	encoder := json.NewEncoder(out)
	for in.Scan() {
		var text string
		if err = json.Unmarshal(in.Bytes(), &text); err != nil {
			return err
		}
		result := text
		if text != "" {
			input := append([]byte(text), 0)
			p, _, _ := syscall.SyscallN(procs["SherpaOfflinePunctuationAddPunct"], handle, uintptr(unsafe.Pointer(&input[0])))
			runtime.KeepAlive(input)
			if p == 0 {
				return fmt.Errorf("punctuation failed")
			}
			// p is memory of the library, not of Go; read it as a C string.
			result = windows.BytePtrToString(*(**byte)(unsafe.Pointer(&p)))
			syscall.SyscallN(procs["SherpaOfflinePunctuationFreeText"], p)
		}
		if err = encoder.Encode(result); err != nil {
			return err
		}
	}
	return in.Err()
}

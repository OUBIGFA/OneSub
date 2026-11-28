//go:build windows

package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"golang.org/x/sys/windows"
)

// These layouts are the pinned sherpa-onnx v2.0.3 C ABI, including padding
// around Pyannote's window ratio. Inference stays outside the UI process.
type diarizeSegConfig struct {
	Model          *byte
	Shift          float32
	Padding        int32
	Threads, Debug int32
	Provider       *byte
}
type diarizeEmbeddingConfig struct {
	Model          *byte
	Threads, Debug int32
	Provider       *byte
}
type diarizeConfig struct {
	Seg           diarizeSegConfig
	Embedding     diarizeEmbeddingConfig
	Count         int32
	Threshold     float32
	Confidence    int32
	MinOn, MinOff float32
}
type diarizeSegment struct {
	Start, End float32
	Speaker    int32
	Confidence float32
}

func runDiarize(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: diarize RUNTIME")
	}
	var request diarizeRequest
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 65536)).Decode(&request); err != nil {
		return err
	}
	if request.Count < 0 || request.Count > 100 {
		return errors.New("speaker count must be between 0 and 100")
	}
	pcm, err := openPCM(request.WAV)
	if err != nil {
		return err
	}
	defer pcm.Close()
	if pcm.length <= 0 || pcm.length/2 > math.MaxInt32 {
		return errors.New("audio exceeds diarization sample limit")
	}
	// Conservative admission budget includes audio, frame scores and clustering.
	var memory struct {
		Length, Load                                                                         uint32
		Total, Available, TotalPage, AvailablePage, TotalVirtual, AvailableVirtual, Extended uint64
	}
	memory.Length = uint32(unsafe.Sizeof(memory))
	ok, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&memory)))
	if ok == 0 || uint64(pcm.length)*12+512*1024*1024 > memory.Available {
		return errors.New("insufficient free memory for whole-file speaker analysis")
	}
	samples := make([]float32, pcm.length/2)
	buffer := make([]byte, 128*1024)
	for offset := int64(0); offset < pcm.length; {
		n := min(int64(len(buffer)), pcm.length-offset)
		if _, err = pcm.file.ReadAt(buffer[:n], pcm.data+offset); err != nil {
			return err
		}
		for i := int64(0); i < n; i += 2 {
			samples[(offset+i)/2] = float32(int16(binary.LittleEndian.Uint16(buffer[i:]))) / 32768
		}
		offset += n
	}
	var dll windows.Handle
	for _, name := range []string{"onnxruntime.dll", "sherpa-onnx-c-api.dll"} {
		dll, err = windows.LoadLibraryEx(filepath.Join(args[0], name), 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
		if err != nil {
			return err
		}
	}
	procs := map[string]uintptr{}
	for _, suffix := range []string{"CreateOfflineSpeakerDiarization", "DestroyOfflineSpeakerDiarization", "OfflineSpeakerDiarizationProcessWithCallbackNoArg", "OfflineSpeakerDiarizationDestroyResult", "OfflineSpeakerDiarizationResultGetNumSegments", "OfflineSpeakerDiarizationResultSortByStartTime", "OfflineSpeakerDiarizationDestroySegment"} {
		name := "SherpaOnnx" + suffix
		procs[suffix], err = windows.GetProcAddress(dll, name)
		if err != nil {
			return err
		}
	}
	if err = os.Chdir(request.Root); err != nil {
		return err
	}
	seg, _ := windows.BytePtrFromString("models/speaker-segmentation/model.onnx")
	embed, _ := windows.BytePtrFromString("models/speaker-embedding/model.onnx")
	cpu, _ := windows.BytePtrFromString("cpu")
	threads := int32(min(4, max(1, runtime.NumCPU()/2)))
	config := diarizeConfig{Seg: diarizeSegConfig{Model: seg, Threads: threads, Provider: cpu}, Embedding: diarizeEmbeddingConfig{Model: embed, Threads: threads, Provider: cpu}, Count: int32(request.Count), Threshold: 0.5, Confidence: 1, MinOn: 0.1, MinOff: 0.1}
	handle, _, _ := syscall.SyscallN(procs["CreateOfflineSpeakerDiarization"], uintptr(unsafe.Pointer(&config)))
	runtime.KeepAlive(config)
	if handle == 0 {
		return errors.New("speaker models could not be loaded")
	}
	defer syscall.SyscallN(procs["DestroyOfflineSpeakerDiarization"], handle)
	callback := syscall.NewCallback(func(done, total int32) uintptr { fmt.Printf("{\"done\":%d,\"total\":%d}\n", done, total); return 0 })
	result, _, _ := syscall.SyscallN(procs["OfflineSpeakerDiarizationProcessWithCallbackNoArg"], handle, uintptr(unsafe.Pointer(&samples[0])), uintptr(len(samples)), callback)
	runtime.KeepAlive(samples)
	if result == 0 {
		return errors.New("speaker analysis failed")
	}
	defer syscall.SyscallN(procs["OfflineSpeakerDiarizationDestroyResult"], result)
	count, _, _ := syscall.SyscallN(procs["OfflineSpeakerDiarizationResultGetNumSegments"], result)
	if count > 1000000 {
		return errors.New("speaker result exceeds limit")
	}
	state := subtitle.SpeakerState{Names: map[string]string{}}
	if count > 0 {
		ptr, _, _ := syscall.SyscallN(procs["OfflineSpeakerDiarizationResultSortByStartTime"], result)
		if ptr == 0 {
			return errors.New("missing speaker intervals")
		}
		defer syscall.SyscallN(procs["OfflineSpeakerDiarizationDestroySegment"], ptr)
		rows := unsafe.Slice((*diarizeSegment)(unsafe.Pointer(ptr)), int(count))
		labels := map[int32]string{}
		for _, row := range rows {
			if math.IsNaN(float64(row.Start)) || math.IsNaN(float64(row.End)) || row.Start < 0 || row.End <= row.Start {
				return errors.New("invalid speaker clock")
			}
			id, exists := labels[row.Speaker]
			if !exists {
				id = fmt.Sprintf("speaker-%d", len(labels)+1)
				labels[row.Speaker] = id
				state.Names[id] = fmt.Sprintf("%c", 'A'+len(labels)-1)
			}
			turn := subtitle.SpeakerTurn{StartMs: int64(math.Round(float64(row.Start) * 1000)), EndMs: int64(math.Round(float64(row.End) * 1000)), ID: id}
			if turn.EndMs > pcm.length/pcmBytesPerMs+1 {
				return errors.New("speaker interval exceeds audio duration")
			}
			if row.Confidence >= -1 && row.Confidence <= 1 {
				score := float64(row.Confidence)
				turn.Score = &score
			}
			state.Turns = append(state.Turns, turn)
		}
	}
	if err = subtitle.ValidateSpeakerState(&state); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(request.Output, raw, 0600)
}

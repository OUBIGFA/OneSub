package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

const (
	punctuateArgument = "punctuate"
	// sherpaChunkMs is the most speech one sherpa-onnx input holds.
	// SenseVoice and Paraformer are trained on utterances under 30 s and
	// degrade on longer input; Parakeet-TDT also needs less memory.
	sherpaChunkMs = 20000
	// sherpaBatch is how many inputs one CLI run decodes together. The CLI
	// loads the model once per run and decodes all its inputs at once.
	sherpaBatch = 16
)

// sherpaRuntime is the folder of the pinned sherpa-onnx CLI and libraries.
func (t toolset) sherpaRuntime() string {
	if value := os.Getenv("ONESUB_SHERPA_ROOT"); value != "" {
		return value
	}
	return filepath.Join(t.resources, "sherpa-onnx")
}

// recognizeWithSherpa is recognize for a sherpa-onnx model: the same audio
// extraction and FireRedVAD speech detection as OneAsr, then the model's
// own token times become word timings and sentence cues. It returns the
// same SRT and word artifacts, in a scratch folder that cleanup removes.
func (t toolset) recognizeWithSherpa(ctx context.Context, asr models.Model, asrDir, vad, language, source string, step func(recognitionStep)) (srt string, words []byte, cleanup func(), err error) {
	scratch, err := os.MkdirTemp(t.worker.Scratch, "asr-")
	if err != nil {
		return "", nil, nil, err
	}
	removeScratch := func() { os.RemoveAll(scratch) }
	defer func() {
		if err != nil {
			removeScratch()
		}
	}()
	wav := filepath.Join(scratch, "input.wav")
	step(recognitionStep{Stage: "extract"})
	args := []string{"-v", "error", "-i", source, "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", wav}
	if err = jobs.Run(ctx, jobs.CommandContext(ctx, t.ffmpeg, args...), nil); err != nil {
		return "", nil, nil, apperr.Wrap(locales.ErrAudioExtract(err.Error(), ""), err)
	}
	step(recognitionStep{Stage: "vad"})
	regions, err := detectSpeech(ctx, vad, wav, sherpaChunkMs)
	if err != nil {
		return "", nil, nil, err
	}
	if len(regions) == 0 {
		return "", nil, nil, errors.New(locales.RecognitionNoSpeech())
	}
	recognized, err := t.recognizeSherpa(ctx, asr, asrDir, language, wav, scratch, regions, step)
	if err != nil {
		return "", nil, nil, err
	}
	if len(recognized) == 0 {
		// Every region held only music, noise or an audio event.
		return "", nil, nil, errors.New(locales.RecognitionNoSpeech())
	}
	step(recognitionStep{Stage: "save"})
	doc := subtitle.Document{Format: "srt", TargetFormat: "srt", Cues: subtitle.WordCues(recognized)}
	all := []subtitle.Word{}
	for i := range doc.Cues {
		doc.Cues[i].ID = fmt.Sprintf("asr-%d", i)
		all = append(all, doc.Cues[i].Words...)
		doc.Cues[i].Words = nil
	}
	data, err := t.worker.Write(ctx, doc)
	if err != nil {
		return "", nil, nil, err
	}
	srt = filepath.Join(scratch, "result.srt")
	if err = os.WriteFile(srt, data, 0600); err != nil {
		return "", nil, nil, err
	}
	if words, err = subtitle.MarshalOneAsrWords(all); err != nil {
		return "", nil, nil, err
	}
	return srt, words, removeScratch, nil
}

// sherpaArgs are the CLI options that load model from dir, a path relative
// to the CLI's working folder.
func sherpaArgs(model models.Model, dir, language string) []string {
	file := func(name string) string { return filepath.ToSlash(filepath.Join(dir, name)) }
	threads := fmt.Sprintf("--num-threads=%d", min(8, max(1, runtime.NumCPU()/2)))
	args := []string{"--tokens=" + file("tokens.txt"), threads}
	switch model.Engine {
	case models.EngineSenseVoice:
		args = append(args, "--sense-voice-model="+file("model.int8.onnx"), "--sense-voice-language="+language, "--sense-voice-use-itn=1")
	case models.EngineParaformer:
		args = append(args, "--paraformer="+file("model.int8.onnx"))
	case models.EngineTransducer:
		args = append(args, "--encoder="+file("encoder.int8.onnx"), "--decoder="+file("decoder.int8.onnx"), "--joiner="+file("joiner.int8.onnx"), "--model-type=nemo_transducer")
	}
	return args
}

// recognizeSherpa recognizes the speech regions of wav with a sherpa-onnx
// model whose files are in modelDir. Each region is cut into inputs of at
// most sherpaChunkMs, every input file and model path the CLI opens is an
// ASCII path relative to scratch (the CLI reads paths in the ANSI code
// page), and the token times come back on the recording's clock.
func (t toolset) recognizeSherpa(ctx context.Context, model models.Model, modelDir, language, wav, scratch string, regions []speechRegion, step func(recognitionStep)) ([]subtitle.SpacedWord, error) {
	root, removeRoot, err := models.RuntimeRoot(ctx, scratch, map[string]string{model.ID: modelDir})
	if err != nil {
		return nil, err
	}
	defer removeRoot()
	rel, err := filepath.Rel(scratch, filepath.Join(root, "models", model.ID))
	if err != nil {
		return nil, err
	}
	pcm, err := openPCM(wav)
	if err != nil {
		return nil, err
	}
	defer pcm.Close()
	names := make([]string, len(regions))
	for i, region := range regions {
		names[i] = fmt.Sprintf("chunk-%06d.wav", i)
		if err = pcm.write(filepath.Join(scratch, names[i]), region); err != nil {
			return nil, err
		}
	}
	style := subtitle.SherpaPieces
	if model.Engine == models.EngineParaformer {
		style = subtitle.SherpaContinuation
	}
	cli := filepath.Join(t.sherpaRuntime(), "sherpa-onnx-offline.exe")
	chunks := make([][]subtitle.SpacedWord, len(regions))
	step(recognitionStep{Stage: "load-asr"})
	for first := 0; first < len(regions); first += sherpaBatch {
		last := min(first+sherpaBatch, len(regions))
		step(recognitionStep{Stage: "transcribe", Done: first + 1, Total: len(regions)})
		command := jobs.CommandContext(ctx, cli, append(sherpaArgs(model, rel, language), names[first:last]...)...)
		command.Dir = scratch
		var output strings.Builder
		if err = jobs.Run(ctx, command, func(line string) {
			if strings.HasPrefix(line, "{") {
				output.WriteString(line)
				output.WriteByte('\n')
			}
		}); err != nil {
			return nil, apperr.Wrap(locales.ErrSherpaRecognition(err.Error(), ""), err)
		}
		results, err := subtitle.ParseSherpaResults(output.String())
		if err != nil {
			return nil, err
		}
		if len(results) != last-first {
			return nil, errors.New(locales.ErrSherpaOutput())
		}
		for i, result := range results {
			region := regions[first+i]
			words := subtitle.SherpaWords(result, style, region.EndMs-region.StartMs)
			for j := range words {
				words[j].StartMs += region.StartMs
				words[j].EndMs += region.StartMs
			}
			chunks[first+i] = words
		}
	}
	if model.Engine == models.EngineParaformer {
		step(recognitionStep{Stage: "punctuate"})
		if err = t.punctuate(ctx, modelDir, chunks); err != nil {
			return nil, err
		}
	}
	all := []subtitle.SpacedWord{}
	for _, words := range chunks {
		if len(words) > 0 && len(all) > 0 {
			// Inputs are separate utterances: a word after another input
			// is spaced by its script, not by the token that began it.
			words[0].Space = subtitle.BoundarySpace(all[len(all)-1].Text, words[0].Text)
		}
		all = append(all, words...)
	}
	return all, nil
}

// punctuate restores the punctuation of Paraformer's words, chunk by chunk,
// with the CT-Transformer model in the model folder. It runs in a child
// OneSub process; see runPunctuate. A chunk whose punctuated text does not
// match its words keeps its words without marks.
func (t toolset) punctuate(ctx context.Context, modelDir string, chunks [][]subtitle.SpacedWord) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	var input strings.Builder
	for _, words := range chunks {
		texts := []string{}
		for _, w := range words {
			texts = append(texts, w.Text)
		}
		line, _ := json.Marshal(strings.Join(texts, " "))
		input.Write(line)
		input.WriteByte('\n')
	}
	command := jobs.CommandContext(ctx, self, punctuateArgument, t.sherpaRuntime(), modelDir)
	command.Stdin = strings.NewReader(input.String())
	outputs := []string{}
	var lineErr error
	if err = jobs.Run(ctx, command, func(line string) {
		var text string
		if err := json.Unmarshal([]byte(line), &text); err != nil {
			lineErr = err
		}
		outputs = append(outputs, text)
	}); err != nil {
		return apperr.Wrap(locales.ErrPunctuation(err.Error()), err)
	}
	if lineErr != nil || len(outputs) != len(chunks) {
		return errors.New(locales.ErrPunctuation(locales.ErrSherpaOutput()))
	}
	for i := range chunks {
		if words, ok := subtitle.PunctuateWords(chunks[i], outputs[i]); ok {
			chunks[i] = words
		}
	}
	return nil
}

// pcmFile is the 16 kHz mono 16-bit PCM WAV that FFmpeg extracted, read by
// sample offset to cut the inputs without another FFmpeg run per input.
type pcmFile struct {
	file         *os.File
	data, length int64
}

const pcmBytesPerMs = 16000 * 2 / 1000

func openPCM(path string) (*pcmFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 12)
	if _, err = io.ReadFull(file, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		file.Close()
		return nil, errors.New(locales.ErrAudioExtract("WAV", ""))
	}
	offset := int64(12)
	var format []byte
	for {
		chunk := make([]byte, 8)
		if _, err = file.ReadAt(chunk, offset); err != nil {
			file.Close()
			return nil, errors.New(locales.ErrAudioExtract("WAV", ""))
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		switch string(chunk[:4]) {
		case "fmt ":
			format = make([]byte, min(size, 16))
			file.ReadAt(format, offset+8)
		case "data":
			if len(format) < 16 || binary.LittleEndian.Uint16(format[0:]) != 1 || binary.LittleEndian.Uint16(format[2:]) != 1 || binary.LittleEndian.Uint32(format[4:]) != 16000 || binary.LittleEndian.Uint16(format[14:]) != 16 {
				file.Close()
				return nil, errors.New(locales.ErrAudioExtract("WAV", ""))
			}
			info, _ := file.Stat()
			return &pcmFile{file: file, data: offset + 8, length: min(size, info.Size()-offset-8)}, nil
		}
		offset += 8 + size + size%2
	}
}

func (p *pcmFile) Close() error { return p.file.Close() }

// write saves region of the recording as its own WAV file.
func (p *pcmFile) write(path string, region speechRegion) error {
	start := min(region.StartMs*pcmBytesPerMs, p.length)
	end := min(region.EndMs*pcmBytesPerMs, p.length)
	samples := make([]byte, end-start)
	if _, err := p.file.ReadAt(samples, p.data+start); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	header := make([]byte, 44)
	copy(header, "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(samples)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], 1)
	binary.LittleEndian.PutUint32(header[24:], 16000)
	binary.LittleEndian.PutUint32(header[28:], 32000)
	binary.LittleEndian.PutUint16(header[32:], 2)
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(samples)))
	return os.WriteFile(path, append(header, samples...), 0600)
}

package media

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/jobs"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/videoencoding"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// hardwareEncoder is one of the bundled FFmpeg's hardware video encoders.
// GPU compression uses the same FFmpeg process, ASS filter and stream plan as
// the CPU path; only the video encoder arguments differ.
type hardwareEncoder struct {
	Name   string // FFmpeg encoder name
	Codec  string // "av1" or "hevc"
	Vendor string // "nvenc", "qsv" or "amf"
	MaxQ   float64
}

// hardwareEncoders is the detection order. AV1 comes first on every vendor,
// so a GPU with AV1 encode (NVIDIA Ada+, Intel Arc/Xe2, AMD RDNA3+) uses it
// automatically; older GPUs fall back to 10-bit HEVC.
var hardwareEncoders = []hardwareEncoder{
	{Name: "av1_nvenc", Codec: "av1", Vendor: "nvenc", MaxQ: 63},
	{Name: "av1_qsv", Codec: "av1", Vendor: "qsv", MaxQ: 51},
	{Name: "av1_amf", Codec: "av1", Vendor: "amf", MaxQ: 51},
	{Name: "hevc_nvenc", Codec: "hevc", Vendor: "nvenc", MaxQ: 51},
	{Name: "hevc_qsv", Codec: "hevc", Vendor: "qsv", MaxQ: 51},
	{Name: "hevc_amf", Codec: "hevc", Vendor: "amf", MaxQ: 51},
}

// videoArgs returns the encoder arguments for the main video stream at
// quality q, always as 10-bit 4:2:0 (p010le) like the CPU path.
//
// NVENC uses P5 + HQ with lookahead and spatial/temporal AQ. On an RTX 3080 Ti
// the earlier UHQ + 2-pass-full setup ran at 62 fps versus about 200-280 fps
// here, with no XPSNR gain at equal bitrate. maxPixelRate (width*height) sets
// an explicit peak rate: without it NVENC derives a level cap (about 7-12
// Mbps at 1080p) that clipped high-quality encodes. QSV uses ICQ, which has
// no peak limit; AMF uses QVBR with the same explicit peak.
func (e hardwareEncoder) videoArgs(q float64, pixels int) []string {
	quality := strconv.Itoa(int(quantizeWithStep(q, 1)))
	// One bit per pixel at 60 fps; far above any quality-limited encode, but
	// keeps a finite VBV for the hardware rate controller.
	maxRate := strconv.Itoa(max(pixels, 640*360) * 60)
	args := []string{"-c:v:0", e.Name}
	switch e.Vendor {
	case "nvenc":
		args = append(args, "-preset", "p5", "-tune", "hq", "-rc", "vbr", "-cq", quality, "-b:v:0", "0",
			"-maxrate:v:0", maxRate, "-bufsize:v:0", maxRate, "-spatial-aq", "1", "-temporal-aq", "1", "-rc-lookahead", "32")
	case "qsv":
		args = append(args, "-preset", "slow", "-global_quality", quality, "-extbrc", "1", "-look_ahead_depth", "40")
	case "amf":
		args = append(args, "-usage", "transcoding", "-quality", "quality", "-rc", "qvbr", "-qvbr_quality_level", quality,
			"-maxrate:v:0", maxRate, "-bufsize:v:0", maxRate)
	}
	return append(args, "-pix_fmt:v:0", "p010le")
}

// pickHardwareEncoder returns the first encoder in detection order that try
// accepts. try runs a real short encode, so a driver or GPU that rejects any
// option moves on to the next candidate instead of failing the user's task.
func pickHardwareEncoder(ctx context.Context, try func(context.Context, hardwareEncoder) error) (hardwareEncoder, error) {
	for _, candidate := range hardwareEncoders {
		if err := ctx.Err(); err != nil {
			return hardwareEncoder{}, err
		}
		if try(ctx, candidate) == nil {
			return candidate, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return hardwareEncoder{}, err
	}
	return hardwareEncoder{}, errors.New(locales.ErrGpuEncoderUnavailable())
}

var detectedEncoders = struct {
	sync.Mutex
	byFFmpeg map[string]hardwareEncoder
}{byFFmpeg: map[string]hardwareEncoder{}}

// DetectGPUEncoder probes the bundled FFmpeg once per process. Only success
// is cached; a missing driver is probed again on the next task.
func DetectGPUEncoder(ctx context.Context, ffmpeg string) (hardwareEncoder, error) {
	detectedEncoders.Lock()
	defer detectedEncoders.Unlock()
	if found, ok := detectedEncoders.byFFmpeg[ffmpeg]; ok {
		return found, nil
	}
	found, err := pickHardwareEncoder(ctx, func(ctx context.Context, candidate hardwareEncoder) error {
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		args := []string{"-hide_banner", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=24:duration=0.5"}
		args = append(args, candidate.videoArgs(candidate.MaxQ/2, 1280*720)...)
		return jobs.CommandContext(probeCtx, ffmpeg, append(args, "-f", "null", "-")...).Run()
	})
	if err != nil {
		return found, err
	}
	detectedEncoders.byFFmpeg[ffmpeg] = found
	return found, nil
}

// gpuPresetFor keeps the CPU preset's XPSNR target and lets the search use
// the encoder's own quality scale. The CPU-equivalent quality floor is
// applied by the search itself (see searchGPUCompressionQuality).
func gpuPresetFor(encoder hardwareEncoder, quality string) (CompressPreset, bool) {
	cpu, ok := CompressPresetFor(quality)
	if !ok {
		return CompressPreset{}, false
	}
	// Starting points measured on hevc_nvenc (1080p): cq 34 ~ XPSNR 44, cq 40
	// ~ 41.7, cq 44 ~ 40. AV1 NVENC uses a 0-63 scale, so it starts higher.
	initial := map[string]float64{videoencoding.QualityNearLossless: 30, videoencoding.QualityHigh: 36, videoencoding.QualityCompact: 42}[quality]
	if encoder.MaxQ > 51 {
		initial += 6
	}
	return CompressPreset{
		ID:          quality,
		TargetXPSNR: cpu.TargetXPSNR,
		MinCRF:      1,
		MaxCRF:      encoder.MaxQ,
		InitialCRF:  initial,
		Step:        1,
	}, true
}

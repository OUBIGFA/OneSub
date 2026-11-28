// Package videoencoding owns persisted encoder and quality IDs. It has no UI,
// settings or process dependencies, so every entry uses the same choices.
package videoencoding

const (
	EncoderCPU          = "cpu"
	EncoderCPUH264      = "cpu-h264"
	EncoderGPU          = "gpu"
	QualityNearLossless = "near-lossless"
	QualityHigh         = "high"
	QualityCompact      = "compact"
	QualityCRF18        = "crf18"
	QualityCRF20        = "crf20"
	QualityCRF23        = "crf23"
)

// Encoders returns a fresh list in the user-facing order.
func Encoders() []string { return []string{EncoderCPU, EncoderCPUH264, EncoderGPU} }

func ValidEncoder(encoder string) bool {
	return encoder == EncoderCPU || encoder == EncoderCPUH264 || encoder == EncoderGPU
}

// Qualities keeps fixed H.264 CRFs separate from measured AV1/GPU targets.
// Unknown encoders expose no options rather than falling back to CPU.
func Qualities(encoder string) []string {
	switch encoder {
	case EncoderCPU, EncoderGPU:
		return []string{QualityNearLossless, QualityHigh, QualityCompact}
	case EncoderCPUH264:
		return []string{QualityCRF18, QualityCRF20, QualityCRF23}
	default:
		return nil
	}
}

func ValidQuality(encoder, quality string) bool {
	for _, candidate := range Qualities(encoder) {
		if candidate == quality {
			return true
		}
	}
	return false
}

// DefaultQuality is the composition/API default. Compression intentionally
// starts H.264 at CRF 23 and persists that preference separately.
func DefaultQuality(encoder string) string {
	if encoder == EncoderCPUH264 {
		return QualityCRF20
	}
	if encoder == EncoderCPU || encoder == EncoderGPU {
		return QualityNearLossless
	}
	return ""
}

// H264CRF resolves a fixed encoder setting, not a perceptual quality claim.
func H264CRF(quality string) (float64, bool) {
	switch quality {
	case QualityCRF18:
		return 18, true
	case QualityCRF20:
		return 20, true
	case QualityCRF23:
		return 23, true
	default:
		return 0, false
	}
}

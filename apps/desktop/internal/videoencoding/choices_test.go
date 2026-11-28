package videoencoding

import "testing"

func TestEncoderQualityContract(t *testing.T) {
	for _, encoder := range Encoders() {
		if !ValidEncoder(encoder) || !ValidQuality(encoder, DefaultQuality(encoder)) {
			t.Fatal("encoder has no usable default", encoder)
		}
		for _, quality := range Qualities(encoder) {
			_, fixed := H264CRF(quality)
			if !ValidQuality(encoder, quality) || fixed != (encoder == EncoderCPUH264) {
				t.Fatal("quality belongs to a different encoding strategy", encoder, quality)
			}
		}
		for _, invalid := range []string{"", "lossless", "crf0", "CRF 20", "invalid"} {
			if ValidQuality(encoder, invalid) {
				t.Fatal("accepted unknown quality", encoder, invalid)
			}
		}
	}
	for _, invalid := range []string{"", "nvenc", "CPU", "unknown"} {
		if ValidEncoder(invalid) || ValidQuality(invalid, QualityNearLossless) || DefaultQuality(invalid) != "" || len(Qualities(invalid)) != 0 {
			t.Fatal("unknown encoder silently became CPU", invalid)
		}
	}
}

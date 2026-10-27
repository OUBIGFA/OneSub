package main

import (
	"bytes"
	"image"
	"image/png"
	"math"

	"golang.org/x/image/draw"
)

// Resample the original artwork once at the taskbar's physical size. The mature
// Catmull-Rom filter preserves alpha edges; Windows never shrinks a 1200px HICON.
func trayLogo(scale float64) ([]byte, error) {
	if scale <= 0 {
		scale = 1
	}
	size := max(16, int(math.Round(16*scale)))
	source, err := png.Decode(bytes.NewReader(appLogo))
	if err != nil {
		return nil, err
	}
	output := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(output, output.Bounds(), source, source.Bounds(), draw.Src, nil)
	var buffer bytes.Buffer
	err = png.Encode(&buffer, output)
	return buffer.Bytes(), err
}

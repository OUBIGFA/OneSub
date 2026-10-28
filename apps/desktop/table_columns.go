package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Measure with the active MyGo font, including its UI scale. Budget the widest
// digit for every place; a proportional digit font must not clip e.g. 8888 when
// the numerically largest row is 10000. Filtering never changes the source max.
func numberColumn(c *ui.Context, maximum int) ui.TableColumn {
	digits := len(strconv.Itoa(max(1, maximum)))
	width := float32(0)
	for digit := '0'; digit <= '9'; digit++ {
		w, _ := c.MeasureText(0, ui.Span{Text: strings.Repeat(string(digit), digits)})
		width = max(width, w)
	}
	header, _ := c.MeasureText(0, ui.Span{Text: "#", Weight: 600})
	width = max(width, header) + 2*c.Theme().Space(2.5) + 2
	return ui.TableColumn{Title: "#", ID: "number", Width: width, MinWidth: width, Fixed: true}
}

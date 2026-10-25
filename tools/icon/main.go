// Command icon draws the application icon (build/appicon.png, 1024x1024).
//
//	cd tools/icon && go run . ../../build/appicon.png
//
// It is a separate module so its drawing dependency stays out of the app.
package main

import (
	"image/color"
	"log"
	"os"

	"github.com/fogleman/gg"
)

const size = 1024

func hex(c uint32, a uint8) color.Color {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: a}
}

func gradient(x0, y0, x1, y1 float64, stops ...color.Color) gg.Gradient {
	g := gg.NewLinearGradient(x0, y0, x1, y1)
	for i, c := range stops {
		g.AddColorStop(float64(i)/float64(len(stops)-1), c)
	}
	return g
}

// shadow fakes a soft drop shadow with stacked translucent rounded rects.
func shadow(dc *gg.Context, x, y, w, h, r, dy, spread float64, alpha uint8) {
	const layers = 18
	for i := layers; i >= 1; i-- {
		grow := spread * float64(i) / layers
		dc.SetColor(hex(0x000000, alpha/uint8(layers)*2/3+1))
		dc.DrawRoundedRectangle(x-grow, y+dy-grow, w+2*grow, h+2*grow, r+grow)
		dc.Fill()
	}
}

func main() {
	out := "appicon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	dc := gg.NewContext(size, size)

	// Rounded-square tile with the macOS icon margin.
	const tx, ty, ts, tr = 100.0, 100.0, 824.0, 190.0
	shadow(dc, tx, ty, ts, ts, tr, 22, 26, 90)
	dc.DrawRoundedRectangle(tx, ty, ts, ts, tr)
	dc.SetFillStyle(gradient(tx, ty, tx+ts, ty+ts, hex(0x4F46E5, 255), hex(0xC026D3, 255), hex(0xF97316, 255)))
	dc.Fill()

	// Soft highlight across the top of the tile.
	dc.Push()
	dc.DrawRoundedRectangle(tx, ty, ts, ts, tr)
	dc.Clip()
	dc.DrawRectangle(tx, ty, ts, ts/2)
	dc.SetFillStyle(gradient(0, ty, 0, ty+ts/2, hex(0xFFFFFF, 70), hex(0xFFFFFF, 0)))
	dc.Fill()
	dc.Pop()
	dc.ResetClip()

	// Markdown mark: an M and a down arrow, centered on the tile.
	mark := func(c color.Color, dy float64) {
		dc.Push()
		dc.Translate(size/2, size/2+dy)
		dc.Scale(1.3, 1.3)
		dc.Translate(-525, -561)
		dc.SetLineWidth(62)
		dc.SetLineCapRound()
		dc.SetLineJoinRound()
		dc.SetColor(c)
		dc.MoveTo(292, 660)
		dc.LineTo(292, 462)
		dc.LineTo(425, 596)
		dc.LineTo(558, 462)
		dc.LineTo(558, 660)
		dc.Stroke()
		dc.MoveTo(692, 462)
		dc.LineTo(692, 656)
		dc.Stroke()
		dc.MoveTo(626, 594)
		dc.LineTo(692, 660)
		dc.LineTo(758, 594)
		dc.Stroke()
		dc.Pop()
	}
	mark(hex(0x000000, 60), 16)
	mark(hex(0xFFFFFF, 255), 0)

	if err := dc.SavePNG(out); err != nil {
		log.Fatal(err)
	}
}

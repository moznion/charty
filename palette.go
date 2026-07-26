package charty

import "image/color"

// The categorical palette. Hues are assigned in this fixed order and never
// cycled: a series keeps its colour no matter how many others are present, so
// adding or filtering a series never repaints the rest. Each slot has a step
// chosen for the light surface and a separate one chosen for the dark surface
// — a dark theme is a different set of steps, not an inverted image.
var (
	seriesLight = []color.RGBA{
		{R: 0x2a, G: 0x78, B: 0xd6, A: 0xff}, // blue
		{R: 0xeb, G: 0x68, B: 0x34, A: 0xff}, // orange
		{R: 0x1b, G: 0xaf, B: 0x7a, A: 0xff}, // aqua
		{R: 0xed, G: 0xa1, B: 0x00, A: 0xff}, // yellow
		{R: 0xe8, G: 0x7b, B: 0xa4, A: 0xff}, // magenta
		{R: 0x00, G: 0x83, B: 0x00, A: 0xff}, // green
		{R: 0x4a, G: 0x3a, B: 0xa7, A: 0xff}, // violet
		{R: 0xe3, G: 0x49, B: 0x48, A: 0xff}, // red
	}
	seriesDark = []string{
		"#3987e5", "#d95926", "#199e70", "#c98500",
		"#d55181", "#008300", "#9085e9", "#e66767",
	}
)

// Chart chrome for the static renderers, which have a single light surface.
var (
	colSurface   = color.RGBA{R: 0xfc, G: 0xfc, B: 0xfb, A: 0xff}
	colGrid      = color.RGBA{R: 0xe1, G: 0xe0, B: 0xd9, A: 0xff}
	colAxis      = color.RGBA{R: 0xc3, G: 0xc2, B: 0xb7, A: 0xff}
	colMuted     = color.RGBA{R: 0x89, G: 0x87, B: 0x81, A: 0xff}
	colPrimary   = color.RGBA{R: 0x0b, G: 0x0b, B: 0x0b, A: 0xff}
	colSecondary = color.RGBA{R: 0x52, G: 0x51, B: 0x4e, A: 0xff}
)

func seriesColor(i int) color.RGBA { return seriesLight[i%len(seriesLight)] }

func seriesHex(i int) string { return seriesDark[i%len(seriesDark)] }

func lightHex(i int) string {
	c := seriesColor(i)
	const hex = "0123456789abcdef"
	return string([]byte{
		'#', hex[c.R>>4], hex[c.R&0xf], hex[c.G>>4], hex[c.G&0xf], hex[c.B>>4], hex[c.B&0xf],
	})
}

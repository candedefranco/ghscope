package ui

import "image/color"

// Sprite is pixel art: each string is a row of pixels, each byte a palette key
// ('.' is transparent). Pixels are drawn with half blocks, two per cell.
type Sprite struct {
	rows    []string
	palette map[byte]color.Color
}

func NewSprite(palette map[byte]color.Color, rows ...string) Sprite {
	return Sprite{rows: rows, palette: palette}
}

func (s Sprite) W() int { return len(s.rows[0]) }
func (s Sprite) H() int { return len(s.rows) } // in pixels (half rows)

// Draw paints the sprite with its top-left pixel at column x, pixel row py.
// clip, if not nil, decides per pixel whether it may be drawn.
func (s Sprite) Draw(cv *Canvas, x, py int, clip func(x, py int) bool) {
	for dy, row := range s.rows {
		for dx := range len(row) {
			col, ok := s.palette[row[dx]]
			if !ok {
				continue
			}
			if clip != nil && !clip(x+dx, py+dy) {
				continue
			}
			cv.Pixel(x+dx, py+dy, col)
		}
	}
}

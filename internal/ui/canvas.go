package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

type cell struct {
	ch     rune
	fg, bg color.Color // nil means the terminal default
	// Pixels: each cell holds two, top and bottom, drawn with half blocks.
	top, bot color.Color
}

// Canvas is a grid of styled cells. The skyline is drawn into it first and
// animations are drawn on top, then Render turns it into a styled string.
type Canvas struct {
	W, H  int
	cells []cell
}

func NewCanvas(w, h int) *Canvas {
	c := &Canvas{W: max(0, w), H: max(0, h)}
	c.cells = make([]cell, c.W*c.H)
	for i := range c.cells {
		c.cells[i].ch = ' '
	}
	return c
}

func (c *Canvas) In(x, y int) bool { return x >= 0 && y >= 0 && x < c.W && y < c.H }

// Set draws a character, keeping the cell's background (so a sprite passing
// in front of a building keeps the wall behind it). Out-of-bounds is ignored.
func (c *Canvas) Set(x, y int, ch rune, fg color.Color) {
	if !c.In(x, y) {
		return
	}
	i := y*c.W + x
	c.cells[i].ch, c.cells[i].fg = ch, fg
}

// SetCell draws a character with an explicit background.
func (c *Canvas) SetCell(x, y int, ch rune, fg, bg color.Color) {
	if !c.In(x, y) {
		return
	}
	c.cells[y*c.W+x] = cell{ch: ch, fg: fg, bg: bg}
}

// Pixel colors one of the two pixels in a cell: py counts half-rows, so the
// canvas is W pixels wide and 2*H pixels tall. Used for sprites.
func (c *Canvas) Pixel(x, py int, col color.Color) {
	y := py / 2
	if py < 0 || !c.In(x, y) {
		return
	}
	if py%2 == 0 {
		c.cells[y*c.W+x].top = col
	} else {
		c.cells[y*c.W+x].bot = col
	}
}

// resolve turns a cell's pixels, if any, into a half-block character.
func (cl cell) resolve() cell {
	switch {
	case cl.top != nil && cl.bot != nil:
		return cell{ch: '▀', fg: cl.top, bg: cl.bot}
	case cl.top != nil:
		return cell{ch: '▀', fg: cl.top, bg: cl.bg}
	case cl.bot != nil:
		return cell{ch: '▄', fg: cl.bot, bg: cl.bg}
	}
	return cl
}

// Text draws a string left to right starting at (x, y).
func (c *Canvas) Text(x, y int, s string, fg color.Color) {
	for i, r := range []rune(s) {
		if r != ' ' {
			c.Set(x+i, y, r, fg)
		}
	}
}

// At returns the character at (x, y), or 0 outside the canvas.
func (c *Canvas) At(x, y int) rune {
	if !c.In(x, y) {
		return 0
	}
	return c.cells[y*c.W+x].resolve().ch
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// Render returns the canvas as lines of styled text, grouping runs of cells
// that share colors so the output stays small.
func (c *Canvas) Render() string {
	var b strings.Builder
	for y := range c.H {
		row := make([]cell, c.W)
		for x := range row {
			row[x] = c.cells[y*c.W+x].resolve()
		}
		for x := 0; x < len(row); {
			start := x
			var run strings.Builder
			for x < len(row) && sameColor(row[x].fg, row[start].fg) && sameColor(row[x].bg, row[start].bg) {
				run.WriteRune(row[x].ch)
				x++
			}
			st := lipgloss.NewStyle()
			if fg := row[start].fg; fg != nil {
				st = st.Foreground(fg)
			}
			if bg := row[start].bg; bg != nil {
				st = st.Background(bg)
			}
			if row[start].fg == nil && row[start].bg == nil {
				b.WriteString(run.String())
			} else {
				b.WriteString(st.Render(run.String()))
			}
		}
		if y < c.H-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

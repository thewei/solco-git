package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// rowFade eases a row's background between rest, hover and selection, so
// the tint arrives and leaves instead of popping; every row built on
// rowTint carries it (colors only: no position or size motion).
var rowFade = ui.ElementTransition{Colors: true, Duration: motionColor}

// splitFileExt cuts a file name before its extension (".json", ".tsx") so a
// long name can give up the middle of its stem and keep the extension; names
// without one (and dotfiles) come back whole.
func splitFileExt(name string) (stem, ext string) {
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 || len(name)-dot > 8 {
		return name, ""
	}
	return name[:dot], name[dot:]
}

// fileNameLabel is a file row's name: when the row is too narrow the stem
// ends in an ellipsis and the extension stays, so session-index.json and
// session-index_test.json no longer both read as "session-in…".
func fileNameLabel(c *ui.Context, name, tooltip string) ui.Element {
	size := Typography().Body
	stem, ext := splitFileExt(name)
	if ext == "" {
		return tipped(c, ui.Text(c, name).FontSize(size).SingleLine().Grow(1).Shrink(1).MinWidth(0), tooltip)
	}
	row := ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Label(name) // one name for screen readers and lookups
	row.Children(func() {
		ui.Text(c, stem).FontSize(size).SingleLine().Shrink(1).MinWidth(0)
		ui.Text(c, ext).FontSize(size).SingleLine().Shrink(0)
	})
	return tipped(c, row, tooltip)
}

// treeRowMotion is rowFade plus the folder motion: a row appearing under an
// already-built list grows in, and one that goes fades out.
var treeRowMotion = ui.ElementTransition{
	Colors:   true,
	Duration: motionRow,
	Enter:    &ui.Motion{Collapse: true},
	Exit:     &ui.Motion{},
}

// rowTint is the shared background policy for selectable rows: selection
// outranks hover, and an untinted row reports ok=false so callers keep the
// surface transparent.
func rowTint(tokens DesignTokens, selected, hovered bool) (ui.Color, bool) {
	switch {
	case selected:
		return tokens.SidebarSelect, true
	case hovered:
		return tokens.SidebarHover, true
	default:
		return ui.Color{}, false
	}
}

// treeRowAction is a row's hover icon button. Tooltip defaults to Label.
// A Menu action turns the icon into a menu button instead of a click: the
// menu is attached to an element that is built every frame (the slot), so a
// chosen item always delivers — a hover-conditional element would be gone
// by the frame after the choice and the choice would be lost.
type treeRowAction struct {
	Label   string
	Tooltip string
	Glyph   *ui.SVG
	Run     func()
	// Menu, when set, is the icon's dropdown; Run stays unused.
	Menu func(*ui.Menu)
}

func (a treeRowAction) tooltip() string {
	if a.Tooltip != "" {
		return a.Tooltip
	}
	return a.Label
}

// sidebarRowHeight is the density-token row height of the sidebar's rows
// (GWB-063): compact 26 / default 30 / comfortable 34 DIP (2026-10-29
// Cursor-3 reskin lifted the default tier). The Git and Files trees take
// treeRowHeight, two tighter.
func (s *Shell) sidebarRowHeight() float32 {
	switch s.sidebarDensity() {
	case DensityCompact:
		return 26
	case DensityComfortable:
		return 34
	default:
		return 30
	}
}

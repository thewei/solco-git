package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The tooltip face is the theme's own card (2026-11-02, replacing the inverted
// nova tip of 2026-10-29 at the user's request: it ignored light/dark and did
// not read as native): the Surface fill, a 1px Border, the row radius, a soft
// shadow, theme text and Kbd chips washed from the text colour, no arrow.
// MyGo keeps the timing (0.6 s rest, 0.4 s warm window), the placement above
// the anchor or below it where there is no room, and the pass-through
// pointer; only the look is ours.
const (
	tipDefaultWidth = 320 // max-w-xs
	tipGap          = 6   // between the anchor and the card
	tipKeyHeight    = 20  // Kbd h-5, and its min-w-5
)

type tipSpec struct {
	keys     []string
	icon     *ui.SVG
	maxWidth float32
}

// tipOption adds to the plain text tooltip.
type tipOption func(*tipSpec)

// tipKeys names the shortcut of the anchor's action as keycap chips after
// the text; shortcuts.go spells them from the binding that really fires.
func tipKeys(keys []string) tipOption { return func(s *tipSpec) { s.keys = keys } }

// tipMaxWidth lets the text run wider than max-w-xs before it wraps — for
// what is long by nature, a directory.
func tipMaxWidth(w float32) tipOption { return func(s *tipSpec) { s.maxWidth = w } }

// tipColors is the tooltip's fill and ink: the theme's own surface and text,
// so the card follows light and dark.
func tipColors(t *ui.Theme) (fill, ink ui.Color) { return t.Surface, t.Text }

// tipShadow is the soft drop under every card, stronger on a dark page.
func tipShadow(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(0, 0, 0, 0.5)
	}
	return ui.RGBA(0, 0, 0, 0.16)
}

// tipped gives anchor the app's tooltip and returns it: text in the nova
// face, with the options' shortcut chips, glyph and width. An empty text
// shows nothing. Like Element.Tooltip it also describes the anchor to
// assistive technology, the shortcut spelled out.
func tipped(c *ui.Context, anchor ui.Element, text string, opts ...tipOption) ui.Element {
	if text == "" {
		return anchor
	}
	spec := tipSpec{maxWidth: tipDefaultWidth}
	for _, opt := range opts {
		opt(&spec)
	}
	description := text
	if len(spec.keys) > 0 {
		description += " (" + strings.Join(spec.keys, "") + ")"
	}
	anchor.Description(description)

	t := c.Theme()
	sp := Spacing()
	fill, ink := tipColors(t)
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		// A Kbd shortens the right padding (has-kbd:pr-1.5).
		right := sp.L
		if len(spec.keys) > 0 {
			right = sp.S
		}
		tip.MaxWidth(spec.maxWidth).Padding(sp.S, right, sp.S, sp.L).Radius(Radius().Row).
			Background(fill).Border(1, t.Border).Shadow(0, 4, 14, 0, tipShadow(t)).
			TextColor(ink).FontSize(Typography().Body)
		tip.AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(tipGap, 0, tipGap, 0)
		tip.Children(func() {
			if spec.icon == nil && len(spec.keys) == 0 {
				ui.Text(c, text)
				return
			}
			ui.Row(c).Gap(sp.S).AlignItems(ui.Center).Children(func() {
				if spec.icon != nil {
					glyph := Controls().IconSmall
					ui.Icon(c, spec.icon).Size(glyph, glyph).TextColor(ink).Shrink(0)
				}
				ui.Text(c, text).Shrink(1).MinWidth(0)
				tipKeycaps(c, spec.keys, t.Dark, ink)
			})
		})
	})
	return anchor
}

// tipKeycaps is shadcn's Kbd group: one chip per key, 20 high and at least
// 20 wide, on a wash of the text colour — a fifth of it on a light card, a
// tenth on a dark one.
func tipKeycaps(c *ui.Context, keys []string, dark bool, ink ui.Color) {
	if len(keys) == 0 {
		return
	}
	sp := Spacing()
	wash := ink.Alpha(0.1)
	if !dark {
		wash = ink.Alpha(0.07)
	}
	ui.Row(c).Gap(sp.XS).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, key := range keys {
			ui.Box(c).Height(tipKeyHeight).MinWidth(tipKeyHeight).Padding(0, sp.XS).Shrink(0).Center().
				Radius(Radius().Control).Background(wash).Children(func() {
				ui.Text(c, key).FontWeight(WeightMedium)
			})
		}
	})
}

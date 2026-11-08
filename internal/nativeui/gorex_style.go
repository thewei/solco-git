package nativeui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// gorexColors is the shared surface palette, in light and dark windows.
type gorexColors struct {
	// cardPaper is the pane card's opaque surface; cardBorder its hairline.
	cardPaper, cardBorder ui.Color
	// cardTranslucent is the pane-card paper when the terminal background
	// itself is transparent (Settings → Terminal): a see-through fill that
	// lets what is behind the window read through the undrawn terminal
	// background.
	cardTranslucent ui.Color

	text, textMuted, textFaint, iconMuted ui.Color
	hover, pressed                        ui.Color

	// tabActive fills a selected icon chip (the surface switch).
	tabActive ui.Color
}

var gorexLightColors = gorexColors{
	cardPaper:       ui.Hex("#ffffff"),
	cardTranslucent: ui.RGBA(255, 255, 255, 0.58),
	cardBorder:      ui.RGBA(0, 0, 0, 0.08),

	text:      ui.Hex("#1d1d1f"),
	textMuted: ui.Hex("#636366"),
	textFaint: ui.Hex("#8e8e93"),
	iconMuted: ui.Hex("#8a8a8f"),
	hover:     ui.RGBA(0, 0, 0, 0.05),
	pressed:   ui.RGBA(0, 0, 0, 0.1),

	// A black wash, not white paper: the light Content is #fcfcfc since the 2026-11-07
	// redesign and a white chip no longer reads on it (toggle_face_test.go).
	tabActive: ui.RGBA(0, 0, 0, 0.08),
}

var gorexDarkColors = gorexColors{
	cardPaper:       ui.Hex("#212121"),
	cardTranslucent: ui.RGBA(33, 33, 33, 0.6),
	cardBorder:      ui.RGBA(255, 255, 255, 0.07),

	text:      ui.Hex("#ececee"),
	textMuted: ui.Hex("#8b8c92"),
	textFaint: ui.Hex("#8e8e93"),
	iconMuted: ui.Hex("#8b8c92"),
	hover:     ui.RGBA(255, 255, 255, 0.06),
	pressed:   ui.RGBA(255, 255, 255, 0.11),

	tabActive: ui.RGBA(255, 255, 255, 0.10),
}

// Increase contrast keeps every surface and strengthens what is drawn on it:
// the card's hairline and the secondary text and icon tones.
var (
	gorexLightHighContrast = func() gorexColors {
		k := gorexLightColors
		k.cardBorder = ui.RGBA(0, 0, 0, 0.35)
		k.textMuted, k.textFaint, k.iconMuted = ui.Hex("#3f3f46"), ui.Hex("#52525b"), ui.Hex("#52525b")
		return k
	}()
	gorexDarkHighContrast = func() gorexColors {
		k := gorexDarkColors
		k.cardBorder = ui.RGBA(255, 255, 255, 0.35)
		k.textMuted, k.textFaint, k.iconMuted = ui.Hex("#d4d4d8"), ui.Hex("#b0b0b8"), ui.Hex("#c4c4cc")
		return k
	}()
)

func gorexColorsOf(dark bool) *gorexColors {
	switch {
	case dark && highContrast.Load():
		return &gorexDarkHighContrast
	case dark:
		return &gorexDarkColors
	case highContrast.Load():
		return &gorexLightHighContrast
	}
	return &gorexLightColors
}

// Metrics of the gorex surfaces. gorexGap is the reference code's single
// spacing unit (its `gap`): the gutter between cards AND around them —
// every other spacing here derives from it.
const (
	gorexGap     = 8
	gorexCardR   = 12 // Radius().Card: pane cards and grouped panels share it
	gorexHeaderH = 34
	// Toolbar buttons are gorexIconBtn with a gorexIconGlyph glyph; the
	// denser pane-header and find-bar buttons use the smaller pair, and the
	// actions that appear inside a hovered list or tree row the smallest.
	gorexIconBtn   = 28
	gorexIconGlyph = 16
	gorexPaneBtn   = 24
	gorexPaneGlyph = 14
	gorexRowBtn    = 20
	gorexRowGlyph  = 12
	// Each card frame insets half the gutter, so adjacent cards keep a
	// full gorexGap between them; the canvas padding adds the other half,
	// completing the edge gutter (edges land on exactly gorexGap too).
	gorexFrameInset = gorexGap / 2
	gorexCardsPad   = gorexGap / 2

	// Split-divider drag gestures dispatch pane.resize in ratio points.
	// Gestures below the floor keep accumulating instead of spamming the
	// daemon with invisible nudges; a single dispatch never exceeds the
	// cap (the daemon itself clamps ratios to [0.1, 0.9]).
	resizeGestureFloor = float32(0.005)
	resizeDispatchCap  = float32(0.8)
)

// gorexContentCard hosts one primary content surface (Diff, Chat, History,
// Files, Settings) as the flat page itself: full-bleed in the content column,
// on the canvas color. There is no floating card, border or shadow around a
// page.
func gorexContentCard(c *ui.Context, k *gorexColors, build func()) {
	ui.Column(c).Grow(1).MinWidth(0).Clip().
		Background(designTokens(c.Theme().Dark).Content).Children(build)
}

// gorexToggleFace is the one set of state faces a button wears: pressed,
// hovered and — for a toggle that is on — the quiet filled chip. Icon
// buttons, the surface switch and text toggles all take it from here, so
// "on" reads the same on every page (2026-10-14 highlight audit: toggles had
// grown a blue tint, an accent tint and this chip). List and tree rows keep
// the sidebar selection tint instead — a row is selected, a button is on.
func gorexToggleFace(b ui.Element, k *gorexColors, on bool) {
	switch {
	case b.Pressed():
		b.Background(k.pressed)
	case b.Hovered():
		b.Background(k.hover)
	case on:
		b.Background(k.tabActive)
	}
}

// iconButtonRadius is the corner of an icon button this many points on a
// side: a third of it, capped at the row radius — shadcn's icon-sm and
// icon-xs both round to 8, so the 28 and 24 tiers share Radius().Row and the
// 20 row tier keeps proportionally round (2026-10-29; it was size/2.6, which
// made the 28 button 10.8). Every shared icon-button face takes its corner
// from here and nowhere else.
func iconButtonRadius(size float32) float32 {
	return min(Radius().Row, size/3)
}

// gorexIconButton is a borderless button of an icon, with a face on hover;
// label names it for screen readers and its tooltip explains it. Selected
// keeps a quiet filled face, as the surface switch's current side does.
func gorexIconButton(c *ui.Context, k *gorexColors, svg *ui.SVG, label, tooltip string, selected bool, size, iconSize float32) ui.Element {
	// A Tab stop (Enter and Space press it), but a pointer press leaves the
	// keyboard focus where it was: the commit field or search box a button
	// sits beside keeps typing (2026-11-08: the shared icon buttons were not
	// Tab stops, so refresh, search and close were mouse-only).
	b := ui.Box(c).Size(size, size).Center().Radius(iconButtonRadius(size)).Cursor(ui.CursorPointer).Role(ui.RoleButton).Label(label).
		Focusable().KeepFocus()
	return gorexIconFace(c, b, k, svg, tooltip, selected, iconSize)
}

// gorexIconButtonDisabled is the icon button with nothing to do (Back with no
// history behind it): the glyph at half strength and no hover face, as
// shadcn's disabled:opacity-50. The tooltip stays, so the shortcut still
// reads where the button would have led.
func gorexIconButtonDisabled(c *ui.Context, k *gorexColors, svg *ui.SVG, label, tooltip string, size, iconSize float32) ui.Element {
	b := tipped(c, ui.Box(c).Size(size, size).Center().Radius(iconButtonRadius(size)).Role(ui.RoleButton).Label(label), tooltip)
	b.Children(func() {
		ui.Icon(c, svg).Size(iconSize, iconSize).TextColor(k.iconMuted.Alpha(0.5))
	})
	return b
}

// gorexIconFace dresses an icon button's box: tooltip, state face, and the
// glyph that darkens on hover or when on.
func gorexIconFace(c *ui.Context, b ui.Element, k *gorexColors, svg *ui.SVG, tooltip string, selected bool, iconSize float32) ui.Element {
	if tooltip != "" {
		tipped(c, b, tooltip)
	}
	gorexToggleFace(b, k, selected)
	b.Transition(ui.ElementTransition{Colors: true, Duration: motionColor})
	col := k.iconMuted
	if b.Hovered() || selected {
		col = k.text
	}
	b.Children(func() {
		ui.Icon(c, svg).Size(iconSize, iconSize).TextColor(col)
	})
	return b
}

// terminalThemeDefault names the follow-appearance gorex palettes in the
// theme picker; the settings store keeps "".
const terminalThemeDefault = "Default (follow window)"

// terminalThemeDisplay maps a stored theme name to the picker's display
// value: the picker needs a concrete string, the store keeps "".
func terminalThemeDisplay(name string) string {
	if name == "" {
		return terminalThemeDefault
	}
	return name
}

// tildePath shortens an absolute path under the home directory to a ~
// form, as pane headers and path rows show it.
func tildePath(path string) string {
	if path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" || !filepath.IsAbs(path) {
		return path
	}
	if path == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join("~", rel)
	}
	return path
}

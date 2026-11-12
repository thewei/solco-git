package nativeui

import (
	"github.com/egoist/mygo/ui"
)

// Readable column widths. A page's content never stretches past its cap: a
// wide window grows equal margins on both sides instead of long lines or a
// one-sided gap. The caps live here, once.
const (
	// formColumnWidth bounds Settings pages, inspectors and the commit form.
	formColumnWidth = settingsContentMaxWidth
	// listColumnWidth bounds list pages (Sessions): rows need more room than
	// a form to carry their metrics column.
	listColumnWidth float32 = 1040
)

// contentColumn lays body out in a column at most maxWidth wide, centered in
// whatever width the page has. A narrower page simply fills it.
func contentColumn(c *ui.Context, maxWidth float32, body func()) {
	ui.Column(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(maxWidth).Children(body)
	})
}

// segmentedTrack dresses the track of a ui.SegmentedBase in ui.Segmented's own
// look — surface fill, 1px outline, concentric radii, segments stretched to
// the track's height — and segmentFace one of its segments: the chosen one a
// chip raised from the track (the page color on the light theme's white track,
// the pressed surface on the dark one — what ui.Segmented itself paints), the
// hovered one a wash. A segmented control whose segments are icons or counts
// (ui.Segmented takes labels only) is built from these two, so every segmented
// control in the app shares one face and one height; segmentedControl is the
// labels-only one.
func segmentedTrack(c *ui.Context, tabs ui.SegmentedParts, label string) {
	t := c.Theme()
	tabs.Track.Label(label).AlignItems(ui.Stretch).Shrink(0).Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border)
}

func segmentFace(c *ui.Context, tabs ui.SegmentedParts, i int, chosen bool, label string) ui.Element {
	return segmentFacePadded(c, tabs, i, chosen, label, c.Theme().Space(3))
}

// segmentFacePadded is segmentFace with the segment's horizontal padding named,
// for a switch whose segments carry a count beside the label and need the
// narrower end of the same face.
func segmentFacePadded(c *ui.Context, tabs ui.SegmentedParts, i int, chosen bool, label string, padX float32) ui.Element {
	t := c.Theme()
	seg := tabs.Segment(i).Padding(t.Space(1), padX).Gap(Spacing().S).
		Radius(t.Radius - t.Space(0.5)).AlignItems(ui.Center).Label(label)
	switch {
	case chosen:
		chip := t.Background
		if t.Dark {
			chip = t.SurfacePressed
		}
		seg.Background(chip).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
	case seg.Hovered():
		seg.Background(t.SurfaceHover)
	}
	return seg
}

// The sidebar has no fill of its own — the window's vibrancy shows through it —
// so what sits on it cannot wear segmentedTrack's opaque face: a white box in
// the light window and a dark slab in the dark one (2026-11-08, user report: the
// view tabs "do not blend into the translucent sidebar"). Controls there take
// the sidebar's own washes instead — SidebarSelect for the chosen segment, as a
// selected row reads, SidebarHover under the pointer — and the muted tone made
// for text on material. materialFace is the one place those are chosen; the
// two-way switches add a hairline track, the view tabs none.
//
//	materialSegmentedTrack + materialSegmentFacePadded: a labeled switch
//	materialNavTrack + materialNavRow: the view rows (frame.go)
func materialFace(c *ui.Context, seg ui.Element, chosen bool) ui.Color {
	t := c.Theme()
	tokens := designTokens(t.Dark)
	ink := sidebarMuted(t)
	switch {
	case seg.Pressed():
		seg.Background(tokens.SidebarSelectStrong)
		ink = t.Text
	case chosen:
		seg.Background(tokens.SidebarSelect)
		ink = t.Text
	case seg.Hovered():
		seg.Background(tokens.SidebarHover)
		ink = t.Text
	}
	seg.Transition(ui.ElementTransition{Colors: true, Duration: motionColor})
	return ink
}

// materialSegmentedTrack is segmentedTrack for a switch on the sidebar: no fill,
// the line made for material as its outline.
func materialSegmentedTrack(c *ui.Context, tabs ui.SegmentedParts, label string) {
	t := c.Theme()
	tabs.Track.Label(label).AlignItems(ui.Stretch).Shrink(0).Padding(t.Space(0.5)).Gap(t.Space(0.5)).
		Radius(t.Radius).Border(1, designTokens(t.Dark).BorderSubtle)
}

// materialSegmentFacePadded is segmentFacePadded for a switch on the sidebar;
// it returns the segment and the ink for its content.
func materialSegmentFacePadded(c *ui.Context, tabs ui.SegmentedParts, i int, chosen bool, label string, padX float32) (ui.Element, ui.Color) {
	t := c.Theme()
	seg := tabs.Segment(i).Padding(t.Space(1), padX).Gap(Spacing().S).
		Radius(t.Radius - t.Space(0.5)).AlignItems(ui.Center).Label(label)
	return seg, materialFace(c, seg, chosen)
}

// materialNavGroup is a column of materialNavRows: a radio group on the
// material with nothing of its own — the chosen row carries the wash.
func materialNavGroup(c *ui.Context, label string, rows func()) ui.Element {
	return ui.RadioGroup(c, rows).Label(label).Gap(Spacing().XXS).AlignItems(ui.Stretch).Shrink(0)
}

// materialNavRow is one row of a materialNavGroup choosing i into *selected: as
// tall and as cornered as the sidebar's tree rows, glyph and name from the left
// edge; it returns the row and the ink for its content. The caller fills it.
func materialNavRow(c *ui.Context, selected *int, i int, label string, height float32) (ui.Element, ui.Color) {
	row := ui.RadioBase(c, selected, i).Height(height).Padding(0, 8).Gap(Spacing().S).
		Radius(treeRowRadius).AlignItems(ui.Center).Label(label)
	return row, materialFace(c, row, *selected == i)
}

// settingsRow is one line of a settingsGroup: an optional lead (an icon or
// brand mark), the label with an optional quiet note, then the trailing
// control. An attached row continues the row above it, with no divider.
type settingsRow struct {
	lead        func()
	label, note string
	control     func()
	attached    bool
}

// row builds a settingsRow.
func row(label, note string, control func()) settingsRow {
	return settingsRow{label: label, note: note, control: control}
}

// captionedField puts a visible caption above a dialog's input and, when
// given, a muted hint under it. A placeholder vanishes the moment the user
// types, so it cannot be the only thing that says what a field is for.
func captionedField(c *ui.Context, caption, hint string, field func()) {
	ui.Column(c).Gap(Spacing().XS).Children(func() {
		ui.Text(c, caption).FontSize(Typography().BodySmall).FontWeight(WeightMedium)
		field()
		if hint != "" {
			ui.Text(c, hint).FontSize(Typography().Caption).TextColor(c.Theme().TextMuted)
		}
	})
}

// quietNote is a list's "nothing here" line and busyNote its "still reading"
// line: one size, one colour and one inset for every Git list and dialog, so
// an empty or loading state reads the same wherever it appears.
func quietNote(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(Typography().Body).TextColor(c.Theme().TextMuted).Padding(12)
}

func busyNote(c *ui.Context, text string) {
	ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
		ui.Spinner(c).Size(14, 14)
		ui.Text(c, text).FontSize(Typography().Body).TextColor(c.Theme().TextMuted)
	})
}

// compactButton is the secondary action of dense surfaces — Git toolbars,
// list rows, popover menus: a regular Button at the footnote size. Surfaces
// never restate that size on a Button; the dense tier changes here, once.
func compactButton(c *ui.Context, label string) ui.Element {
	return regularButton(c, label).FontSize(Typography().Micro).Height(Controls().CompactHeight)
}

// compactPrimaryButton is compactButton's accent-filled form.
func compactPrimaryButton(c *ui.Context, label string) ui.Element {
	return regularPrimaryButton(c, label).FontSize(Typography().Micro).Height(Controls().CompactHeight)
}

// emptyState is the shared nothing-here state. An action, when given, is the
// way out (a button under the text).
func emptyState(c *ui.Context, title, detail string, action ...func()) {
	t := c.Theme()
	typ := Typography()
	sp := Spacing()
	// The side inset keeps a wrapping detail off the edges of a narrow column
	// (the right panel's card); a wide center never notices it.
	ui.Column(c).Grow(1).Center().Gap(sp.XS).Padding(0, sp.L).Children(func() {
		ui.Text(c, title).FontWeight(WeightSemibold)
		if detail != "" {
			ui.Text(c, detail).FontSize(typ.BodySmall).TextColor(t.TextMuted)
		}
		for _, build := range action {
			ui.Box(c).Height(sp.XS).Shrink(0).PassThrough() // the button sits a step apart from the text
			build()
		}
	})
}

// workspaceEmptyBanner is a compact, top-aligned empty state for the
// Terminal-adjacent Chat/Diff surfaces. It uses the same layout and tokens
// everywhere, instead of a centered modal-like card in the main canvas.
func workspaceEmptyBanner(c *ui.Context, glyph *ui.SVG, title, detail string, actions func()) {
	const emptyBannerMessageBasis float32 = 200 // the least width the message keeps before the actions wrap below it
	t := c.Theme()
	tokens := designTokens(t.Dark)
	sp := Spacing()
	ui.Column(c).Grow(1).MinWidth(0).Padding(sp.XL).Children(func() {
		// Wrap + Basis: Grow alone starts the message from no width, so the
		// row never wrapped and a narrow column squeezed it into single
		// letters beside the actions. With a basis the actions drop below it.
		ui.Row(c).FillWidth().Gap(sp.L).Padding(sp.L).Wrap().
			BorderWidth(0, 0, 1, 0).BorderColor(tokens.BorderSubtle).
			AlignItems(ui.Center).Children(func() {
			ui.Icon(c, glyph).Size(18, 18).TextColor(t.TextMuted).Shrink(0)
			ui.Column(c).Grow(1).Basis(emptyBannerMessageBasis).MinWidth(0).Gap(sp.XS).Children(func() {
				ui.Text(c, title).FontSize(Typography().Section).FontWeight(WeightSemibold)
				if detail != "" {
					ui.Text(c, detail).FontSize(Typography().BodySmall).TextColor(t.TextMuted)
				}
			})
			if actions != nil {
				ui.Row(c).Gap(sp.S).AlignItems(ui.Center).Children(actions)
			}
		})
	})
}

// statusPill is the shared semantic status badge: one vocabulary for Agent
// rows, aggregates, Titlebar Activity and future Chat.
func statusPill(c *ui.Context, text string, tone StatusTone) {
	dark := c.Theme().Dark
	tokens := designTokens(dark)
	typ := Typography()
	sp := Spacing()
	color := tokens.StatusColor(tone, dark)
	ui.Row(c).Gap(sp.XXS).AlignItems(ui.Center).
		Radius(Radius().Row).Padding(sp.XXS, sp.S).
		Background(tokens.StatusBackground(tone, dark)).Shrink(0).Children(func() {
		ui.Box(c).Size(6, 6).Radius(3).Background(color).Shrink(0)
		ui.Text(c, text).FontSize(typ.Micro).FontWeight(WeightSemibold).TextColor(color)
	})
}

// rowInset is the horizontal padding of a grouped row; dividers start at it.
const rowInset float32 = 14

// groupLabel is the quiet caption above a card: small, muted, medium weight.
func groupLabel(c *ui.Context, label string) {
	ui.Text(c, label).FontSize(Typography().BodySmall).FontWeight(WeightMedium).
		TextColor(c.Theme().TextMuted).Padding(0, 0, 0, Spacing().XS)
}

// settingsGroup is the grouped list used by every settings-like surface: a
// quiet label above one rounded panel whose rows are split by inset
// hairlines, as in macOS' own Settings.
func settingsGroup(c *ui.Context, title string, rows ...settingsRow) ui.Element {
	sp := Spacing()
	tokens := designTokens(c.Theme().Dark)
	return ui.Column(c).FillWidth().Gap(sp.S).Children(func() {
		if title != "" {
			groupLabel(c, title)
		}
		ui.Column(c).FillWidth().Radius(Radius().Card).Background(tokens.Panel).
			Border(1, tokens.BorderSubtle).Clip().Children(func() {
			for i, r := range rows {
				if i > 0 && !r.attached {
					ui.Row(c).FillWidth().Height(1).Shrink(0).Children(func() {
						ui.Box(c).Width(rowInset).Shrink(0)
						ui.Box(c).Grow(1).FillHeight().Background(tokens.BorderSubtle)
					})
				}
				formRowLead(c, r.lead, r.label, r.note, r.control)
			}
		})
	})
}

// formRow is one labeled row: the label leads at regular weight, an optional
// note sits under it in muted caption type, and the control trails.
func formRow(c *ui.Context, label, detail string, control func()) {
	formRowLead(c, nil, label, detail, control)
}

// formRowLead is formRow with a leading icon or mark. The row is built
// control-first and laid out in reverse, so the control precedes its label in
// focus and reading order — the order macOS' own switch and field rows use.
func formRowLead(c *ui.Context, lead func(), label, detail string, control func()) {
	t := c.Theme()
	typ := Typography()
	sp := Spacing()
	ui.Row(c).FillWidth().MinHeight(Controls().RowHeight+8).Padding(sp.M, rowInset).Gap(sp.L).AlignItems(ui.Center).Reverse().Children(func() {
		if control != nil {
			control()
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(typ.Body)
			if detail != "" {
				ui.Text(c, detail).FontSize(typ.Caption).TextColor(t.TextMuted)
			}
		})
		if lead != nil {
			lead()
		}
	})
}

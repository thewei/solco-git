package nativeui

import "github.com/egoist/mygo/ui"

// regularButton is the outline button of the regular tier.
func regularButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label)
}

// regularPrimaryButton is regularButton's accent-filled form. MyGo gives a
// primary no border, which makes it two points shorter than the button beside
// it; nova's carries one, transparent, and so does this.
func regularPrimaryButton(c *ui.Context, label string) ui.Element {
	return ui.PrimaryButton(c, label).Border(1, ui.Transparent)
}

// textField is the single-line text input.
func textField(c *ui.Context, value *string) ui.Element {
	return ui.TextInput(c, value)
}

// textArea is the multi-line text input.
func textArea(c *ui.Context, value *string) ui.Element {
	return ui.TextArea(c, value)
}

// searchField is the field for searching and filtering: a magnifier, the text
// and a clear button around a text input.
func searchField(c *ui.Context, query *string) ui.Element {
	return ui.SearchField(c, query)
}

// selectField is the drop-down choosing one of options.
func selectField(c *ui.Context, selected *string, options []string) ui.Element {
	return ui.Select(c, selected, options)
}

// segmentedControl is the switch between a few views, labels only; icons and
// counts take segmentedTrack/segmentFace around ui.SegmentedBase directly.
// Both wear ui.Segmented's own look, which TestSegmentedControlMatchesTheOfficialFace
// pins pixel for pixel. A segment is named by its text, as ui.Segmented's are,
// so it takes no label of its own.
func segmentedControl(c *ui.Context, selected *int, labels ...string) ui.Element {
	tabs := ui.SegmentedBase(c, selected, len(labels))
	segmentedTrack(c, tabs, "")
	tabs.Track.Children(func() {
		for i, label := range labels {
			segmentFace(c, tabs, i, i == *selected, "").Children(func() {
				ui.Text(c, label).SingleLine()
			})
		}
	})
	return tabs.Track
}

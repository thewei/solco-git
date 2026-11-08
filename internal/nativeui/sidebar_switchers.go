package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// headerSearch is the search a sidebar head may carry: a magnifier button
// that opens a field under the head. Closing it (the button again, or Esc)
// clears the query, so no hidden filter outlives its field.
type headerSearch struct {
	Query *string
	Open  *bool
	// Label names the field; Button names the magnifier that opens it.
	Label, Button string
	// Changed runs when the query is edited or cleared by closing.
	Changed func()
	// Submit runs on Enter; nil leaves Enter to the field.
	Submit func()
}

// close empties the search and hides its field.
func (h *headerSearch) close() {
	*h.Open = false
	if *h.Query != "" {
		*h.Query = ""
		if h.Changed != nil {
			h.Changed()
		}
	}
}

// button is the magnifier that opens and closes the field; it wears the
// selected face while the field shows.
func (h *headerSearch) button(c *ui.Context, k *gorexColors) {
	label := h.Button
	if *h.Open {
		label = "Close " + strings.ToLower(h.Button)
	}
	if gorexIconButton(c, k, iconSearch, label, label, *h.Open, gorexPaneBtn, gorexPaneGlyph).Clicked() {
		if *h.Open {
			h.close()
		} else {
			*h.Open = true
		}
	}
}

// field is the opened search: focused as it appears, Esc closes it.
func (h *headerSearch) field(c *ui.Context) {
	f := searchField(c, h.Query).FillWidth().AutoFocus().Label(h.Label)
	if f.Changed() && h.Changed != nil {
		h.Changed()
	}
	if f.Submitted() && h.Submit != nil {
		h.Submit()
	}
	if f.Shortcut(0, ui.KeyEscape) {
		h.close()
	}
}

package nativeui

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

// The desktop calls below are MyGo's own (Clipboard and Shell), vars so that
// tests, which run no application loop for them to wait on, never reach them.
var (
	writeClipboard = func(text string) error { return mygo.Clipboard.Write(transfer.TextData(text)) }
	revealInFinder = mygo.Shell.ShowItemInFolder
)

// copyTextToClipboard puts text on the system clipboard. MyGo writes the
// pasteboard itself: a pbcopy child of a Finder-launched app has no UTF-8
// locale and turns CJK text into mojibake.
func (s *Shell) copyTextToClipboard(text string) {
	if err := writeClipboard(text); err != nil {
		s.status = "Clipboard unavailable"
	}
}

// openExternalPathParent reveals a path in Finder, reporting one that is gone
// through the same status channel as its clipboard sibling instead of
// silently doing nothing (F175).
func (s *Shell) openExternalPathParent(path string) {
	if _, err := os.Lstat(path); err != nil {
		s.status = "Could not reveal the file in Finder"
		return
	}
	revealInFinder(path)
}

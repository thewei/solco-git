package nativeui

import (
	"github.com/egoist/mygo/ui"
)

// showViewFiles and goFiles are the Files view's two spellings: Files has no
// back-to-terminal toggle here, naming it while it is open keeps it.
func (s *Shell) showViewFiles() { s.showView(viewFiles) }
func (s *Shell) goFiles()       { s.showView(viewFiles) }

// filesPage is the preview center: the open file's bar over its tier. The
// title bar names the page and the folder it browses and carries the open
// file's Actions (Copy Path, Reveal in Finder — route_header.go); the
// shared left sidebar renders directory navigation and picks the file; the
// plan (files_native_preview.go) decides whether the native code view, the
// native bitmap or the WebView page shows it. This page never edits.
func (s *Shell) filesPage(c *ui.Context) {
	s.syncRightPanelRoot()
	st := &s.rightPanel
	root := st.currentRoot
	selected := st.selectedFilePath
	ui.Column(c).FillWidth().Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		switch {
		case root == "":
			emptyState(c, "No folder", "Open a repository to browse its files.")
		case selected == "":
			s.filesStart(c, root)
		default:
			s.filesPreviewBar(c, selected)
			s.filesPreviewBody(c, selected)
		}
	})
}

// filesPreviewBody is the view the plan chose for the open file. The foot
// (the gutter's legend, language and line count) belongs to the code view, so
// formatted Markdown has none.
func (s *Shell) filesPreviewBody(c *ui.Context, selected string) {
	st := &s.rightPanel
	switch {
	case s.blameOn(selected):
		s.filesBlameBody(c, selected)
	case st.previewPlanned != selected:
		// The size probe is still landing.
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Spinner(c).Size(16, 16)
		})
	default:
		s.nativePreviewBody(c)
		if st.nativeMarkdown == nil || st.markdownSource {
			s.filesPreviewFoot(c)
		}
	}
}

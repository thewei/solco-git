package nativeui

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/thewei/solco-git/internal/preview"
)

// filesStaticWindow keys the one Local Preview window the static server's
// pages share.
const filesStaticWindow = "files-static"

// The desktop calls are vars so tests, which run no application loop for
// them to wait on, never reach the real window or browser.
var (
	openInBrowser     = mygo.Shell.OpenExternal
	showStaticPreview = func(s *Shell, url string) error {
		_, err := s.previews.controllerFor(filesStaticWindow).ShowPreviewURL(url)
		return err
	}
)

// isHTMLPage reports whether the file is a page a browser renders.
func isHTMLPage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".htm", ".xhtml":
		return true
	}
	return false
}

// filesStaticURL serves the root (once) and returns the address of path.
func (s *Shell) filesStaticURL(path string) (string, bool) {
	st := &s.rightPanel
	root := st.currentRoot
	rel, err := filepath.Rel(root, path)
	if root == "" || err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		s.status = "The file is outside the project folder"
		return "", false
	}
	if s.staticPreview == nil || s.staticPreviewRoot != root {
		s.closeStaticPreview()
		srv, err := preview.StartStatic(root)
		if err != nil {
			s.status = "Could not start the preview server"
			return "", false
		}
		s.staticPreview, s.staticPreviewRoot = srv, root
	}
	s.syncStaticPreview()
	return s.staticPreview.URLFor(rel), true
}

// syncStaticPreview hands the server the tree's ignore state: what the
// tree hides, the server does not serve.
func (s *Shell) syncStaticPreview() {
	if s.staticPreview != nil {
		s.staticPreview.SetIgnore(s.rightPanel.ignoredSet, s.rightPanel.showIgnored)
	}
}

// closeStaticPreview stops the server (a new project, or none).
func (s *Shell) closeStaticPreview() {
	if s.staticPreview != nil {
		_ = s.staticPreview.Close()
		s.staticPreview, s.staticPreviewRoot = nil, ""
	}
}

// openStaticPreview shows the page in the Local Preview window.
func (s *Shell) openStaticPreview(path string) {
	url, ok := s.filesStaticURL(path)
	if !ok {
		return
	}
	if err := showStaticPreview(s, url); err != nil {
		s.status = "Preview window unavailable"
		return
	}
	s.status = "Serving " + filepath.Base(s.staticPreviewRoot) + " at " + s.staticPreview.Origin()
}

// openStaticInBrowser hands the served page to the system browser.
func (s *Shell) openStaticInBrowser(path string) {
	url, ok := s.filesStaticURL(path)
	if !ok {
		return
	}
	if err := openInBrowser(url); err != nil {
		s.status = "Could not open the browser"
	}
}

package nativeui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
)

// openFilePreview records the tree selection and routes it through the
// preview plan.
func (s *Shell) openFilePreview(path string) {
	st := &s.rightPanel
	if st.reveal != nil && st.reveal.path != path {
		st.reveal = nil // a hit's line belongs to its own file only
	}
	if st.selectedFilePath == path {
		return
	}
	// Unsaved edits are asked about before another file takes the view.
	s.guardDirty("open another file", func() {
		st.selectedFilePath = path
		s.noteRecentFile(path)
		s.planFilePreview(path)
	})
}

// planFilePreview picks the selection's tier (planPreviewTier). The size
// probe runs off the render closure; the plan lands on the UI lane.
func (s *Shell) planFilePreview(path string) {
	st := &s.rightPanel
	s.clearNativeFilesPreview()
	root := st.currentRoot
	if s.win == nil {
		// Headless (tests): probe and decide inline like requestDirectory.
		size, flavor := probePreview(path)
		s.applyPreviewPlan(path, root, size, flavor)
		return
	}
	go func() {
		size, flavor := probePreview(path)
		s.callOnWindow(func() { s.applyPreviewPlan(path, root, size, flavor) })
	}()
}

// setPreviewSource switches the sticky Rendered/Source choice. A small
// Markdown file holds both views from one read, so the switch only changes
// which one the body builds; a line a search hit asked for lands now if the
// editor is the view the switch brought up.
func (s *Shell) setPreviewSource(source bool) {
	st := &s.rightPanel
	if st.previewSource == source {
		return
	}
	st.previewSource = source
	s.applyFilesReveal()
}

// probePreview stats the file and reads its flavor off the UI lane.
func probePreview(path string) (int64, filesview.Flavor) {
	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	return size, filesview.PreviewFlavor(path)
}

// applyPreviewPlan routes the selection to its tier (UI lane). A failed
// stat (file just removed) still plans on the name alone; the tiers surface
// their own read errors.
func (s *Shell) applyPreviewPlan(path, root string, size int64, flavor filesview.Flavor) {
	st := &s.rightPanel
	if st.currentRoot != root || st.selectedFilePath != path {
		return // stale: the user moved on while the probe ran
	}
	st.previewSize = size
	st.previewPlanned = path
	switch planPreviewTier(path, size) {
	case tierImage:
		s.beginNativeImage(path)
	case tierWebImage:
		s.beginWebImage(path)
	case tierImageTooLarge:
		s.claimNativePreview(path)
		st.nativeImageFailed = "This image is too large to preview. Reveal it in Finder to open it."
	default: // tierText, tierMarkdown, tierSVG
		s.beginNativeText(path, flavor)
	}
}

// claimNativePreview hands the center to the native tier for path with
// every native slot empty.
func (s *Shell) claimNativePreview(path string) {
	st := &s.rightPanel
	if st.webImage {
		st.webImage = false
		s.dropFilesWebview() // the page and its engine process go with the image
	}
	st.nativePreviewPath = path
	s.editResetFor(path)
	st.nativeText = nil
	st.nativeCode = nil
	st.nativeMarkdown = nil
	st.nativeSVG = nil
	st.previewNote = ""
	st.nativeTextLoading = false
	st.nativeTextFailed = ""
	st.nativeImage = nil
	st.nativeImageLoading = false
	st.nativeImageFailed = ""
}

// beginNativeText loads a text file into the code view; a Markdown file
// under markdownRenderCap also gets its formatted view, and an SVG under
// svgRenderCap its drawing, from the same read. The read, the editor Buffer (a
// rope, safe from any goroutine), the cut of a Markdown file into rows and the
// parse of an SVG all happen off the UI lane; the State that shows it is made
// on it.
func (s *Shell) beginNativeText(path string, flavor filesview.Flavor) {
	st := &s.rightPanel
	s.claimNativePreview(path)
	st.nativeTextLoading = true
	gen := st.filesGen.Load()
	root := st.currentRoot
	name := filepath.Base(path)
	tier := planPreviewTier(path, st.previewSize)
	// The gutter is drawn against the file's version in HEAD; what Git knows
	// is read here, on the UI lane, and the blob itself in the background.
	baseRel, baseNew, wantBase := s.diffBasePlan(path)
	runner := s.git.runner
	gitRoot := s.git.root
	type loaded struct {
		preview  filesview.Preview
		editable bool // whole, valid UTF-8: safe to write back
		buf      *editor.Buffer
		base     *editor.Buffer
		md       *markdownPreview
		svg      *ui.SVG
		note     string // why a file with a second view has only the source
		err      error
	}
	load := func() loaded {
		preview, err := filesview.ReadText(path, root)
		if err != nil || preview.IsBinary {
			return loaded{preview: preview, err: err}
		}
		l := loaded{buf: editor.NewBuffer(preview.Content)}
		l.editable = !preview.Truncated && !strings.ContainsRune(preview.Content, '\uFFFD')
		l.buf.SetLanguage(editor.LanguageFor(name))
		if !preview.Truncated {
			switch tier {
			case tierMarkdown:
				if l.md = newMarkdownPreview(path, root, preview.Content, s.revealFile); l.md == nil {
					l.note = "Too complex to format, showing the source"
				}
			case tierSVG:
				var err error
				if l.svg, err = ui.ParseSVG([]byte(preview.Content)); err != nil {
					l.svg, l.note = nil, "Can't draw this SVG, showing the source"
				}
			}
		}
		preview.Content = "" // the Buffer (and the formatted view) hold the text now
		l.preview = preview
		if wantBase {
			l.base = readDiffBase(runner, gitRoot, baseRel, baseNew)
		}
		return l
	}
	apply := func(l loaded) {
		if gen != st.filesGen.Load() || st.currentRoot != root || st.selectedFilePath != path {
			return
		}
		st.nativeTextLoading = false
		if l.err != nil {
			st.nativeTextFailed = l.err.Error()
			return
		}
		preview := l.preview
		st.nativeText = &preview
		st.nativeMarkdown, st.nativeSVG, st.previewNote = l.md, l.svg, l.note
		if l.buf != nil {
			s.edit.editable = l.editable
			s.edit.sig, _ = sigOf(path)
			st.nativeCode = &editor.State{
				Buffer:       l.buf,
				ReadOnly:     true,
				NoActiveLine: true, // a viewer has no line being worked on
				Label:        name,
				Placeholder:  "Empty file",
				DiffBase:     l.base,
				// Prose wraps; code scrolls sideways.
				SoftWrap: flavor == filesview.FlavorMarkdown || l.buf.Language() == nil,
				// After the editor's own Copy / Select All.
				ContextMenu: func(m *ui.Menu) {
					m.Separator()
					s.filesPathItems(m, path, true)
				},
			}
		}
		s.applyFilesReveal()
	}
	if s.win == nil {
		apply(load())
		return
	}
	go func() {
		l := load()
		s.applyOnWindow(func() { apply(l) })
	}()
}

// beginNativeImage decodes an image for the Native tier: the bounded
// decode runs off the UI lane, the Bitmap wraps on it (texture upload
// wants the main thread).
func (s *Shell) beginNativeImage(path string) {
	st := &s.rightPanel
	s.claimNativePreview(path)
	st.nativeImageLoading = true
	gen := st.filesGen.Load()
	root := st.currentRoot
	if s.win == nil {
		// Headless (tests): decode inline; the Bitmap wrap needs the GPU,
		// so headless state carries only the outcome, never the bitmap.
		_, err := filesview.DecodeImageBounded(path)
		if gen != st.filesGen.Load() || st.currentRoot != root || st.selectedFilePath != path {
			return
		}
		st.nativeImageLoading = false
		if err != nil {
			st.nativeImageFailed = err.Error()
		}
		return
	}
	go func() {
		img, err := filesview.DecodeImageBounded(path)
		s.callOnWindow(func() {
			if gen != st.filesGen.Load() || st.currentRoot != root || st.selectedFilePath != path {
				return
			}
			st.nativeImageLoading = false
			if err != nil {
				st.nativeImageFailed = err.Error()
				return
			}
			st.nativeImage = ui.NewBitmap(img)
		})
	}()
}

// clearNativeFilesPreview drops the per-selection preview state (selection
// change, root change, removal of the selected file); the sticky
// previewSource choice stays.
func (s *Shell) clearNativeFilesPreview() {
	st := &s.rightPanel
	st.previewSize = 0
	st.previewPlanned = ""
	s.claimNativePreview("")
}

// filesPreviewBar is the open file's strip over every tier: name, size,
// the limit that cut it, why a file has only its source, and the
// Rendered|Image / Source choice of a Markdown file or SVG small enough to
// show as one.
func (s *Shell) filesPreviewBar(c *ui.Context, selected string) {
	st := &s.rightPanel
	t := c.Theme()
	typ := Typography()
	sp := Spacing()
	// The segmented choice sets the bar's height; every file keeps it, so
	// the code never jumps between a Markdown file and the rest. Its side
	// padding is the title bar's (XL), so the name sits under the page title.
	ui.Row(c).FillWidth().MinHeight(Controls().RegularHeight+2*sp.S).Padding(sp.S, sp.XL).Gap(sp.M).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(designTokens(t.Dark).BorderSubtle).Children(func() {
		ui.Icon(c, iconFile).Size(15, 15).TextColor(t.TextMuted).Shrink(0)
		tipped(c, ui.Text(c, filepath.Base(selected)).FontSize(typ.Body).FontWeight(WeightSemibold).
			Shrink(1).MinWidth(0).SingleLine(), selected)
		// The folder the file sits in, quiet, takes the room the name leaves.
		folder := ""
		if rel, ok := s.repoRel(selected); ok && filepath.Dir(rel) != "." {
			folder = filepath.Dir(rel)
		}
		ui.Text(c, folder).FontSize(typ.Caption).TextColor(t.TextMuted).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
		if st.previewSize > 0 {
			ui.Text(c, filesview.FormatFileSize(st.previewSize)).
				FontSize(typ.Caption).TextColor(t.TextMuted)
		}
		if st.nativeText != nil && st.nativeText.Truncated {
			statusPill(c, "Showing the first "+filesview.FormatFileSize(filesview.TextPreviewCap), ToneAttention)
		}
		if st.previewNote != "" {
			statusPill(c, st.previewNote, ToneAttention)
		}
		if isHTMLPage(selected) {
			s.syncStaticPreview()
			k := gorexColorsOf(t.Dark)
			if gorexIconButton(c, k, iconGlobe, "Open in Browser", "Open in the default browser", false, gorexPaneBtn, gorexPaneGlyph).Clicked() {
				s.openStaticInBrowser(selected)
			}
			if tipped(c, regularButton(c, "Open Preview"), "Serve the project folder on localhost and preview the page").Clicked() {
				s.openStaticPreview(selected)
			}
		}
		if st.previewPlanned == selected {
			rendered, label := "", ""
			switch {
			case st.nativeMarkdown != nil:
				rendered, label = "Rendered", "Markdown view"
			case st.nativeSVG != nil:
				rendered, label = "Image", "SVG view"
			}
			if rendered != "" && !s.edit.editing { // an edit is of the source
				choice := 0
				if st.previewSource {
					choice = 1
				}
				if segmentedControl(c, &choice, rendered, "Source").Label(label).Changed() {
					s.setPreviewSource(choice == 1)
				}
			}
		}
		s.editControls(c, selected)
		s.filesGitControls(c, selected)
	})
}

// filesWebviewFill is what the AVIF/ICO web view is filled with — before its
// page paints and behind the image. Here the preview sits straight on the
// window canvas (there is no card, unlike Solco's Detail card), which is what
// webviewBackground is.
func filesWebviewFill() string { return webviewBackground }

// previewFileMenu is the right-click menu of an image's or drawing's body: the
// helpers of the open file (the code and Markdown views add them to their own
// menus).
func (s *Shell) previewFileMenu(m *ui.Menu) {
	s.filesPathItems(m, s.rightPanel.nativePreviewPath, true)
}

// nativePreviewBody renders what the plan chose for the center: the formatted
// Markdown, the code view, the decoded image, the SVG drawing, the AVIF/ICO
// page, or the truthful state.
func (s *Shell) nativePreviewBody(c *ui.Context) {
	st := &s.rightPanel
	switch {
	case st.nativeTextLoading || st.nativeImageLoading:
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Spinner(c).Size(16, 16)
		})
	case st.nativeImageFailed != "":
		emptyState(c, "Can't preview this image", st.nativeImageFailed)
	case st.nativeTextFailed != "":
		emptyState(c, "Can't read this file", st.nativeTextFailed)
	case st.nativeImage != nil:
		ui.Box(c).Grow(1).FillWidth().Padding(Spacing().L).ContextMenu(s.previewFileMenu).Children(func() {
			ui.Image(c, st.nativeImage).Fit(ui.ScaleDown).Grow(1).FillWidth()
		})
	case st.webImage:
		s.filesWebviewBody(c)
	case st.nativeText != nil && st.nativeText.IsBinary:
		emptyState(c, "Binary file", "Reveal it in Finder to open it.")
	case st.nativeMarkdown != nil && !st.previewSource:
		s.markdownBody(c, st.nativeMarkdown)
	case st.nativeSVG != nil && !st.previewSource:
		// A drawing is sharp at any size: it fills the area, up or down.
		ui.Box(c).Grow(1).FillWidth().Padding(Spacing().L).ContextMenu(s.previewFileMenu).Children(func() {
			ui.Image(c, st.nativeSVG).Fit(ui.Contain).Grow(1).FillWidth()
		})
	case st.nativeCode != nil:
		// The theme follows the window each frame: an appearance change
		// repaints an open file without reloading it.
		st.nativeCode.Theme = codeEditorTheme(c.Theme())
		ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			s.editBanner(c)
			editor.View(c, st.nativeCode).FillWidth().Grow(1).MinHeight(0)
		})
	default:
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Spinner(c).Size(16, 16)
		})
	}
}

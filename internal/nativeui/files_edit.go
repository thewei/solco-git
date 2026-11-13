package nativeui

/**
 * [INPUT]: files_native_preview.go 的 editor.State（预览里那一个）、filesview.ReadText、gitworkbench 快照（applyGitSnapshot 每次调 syncEditedFile）、ui.AlertDialog
 * [OUTPUT]: Files 里的就地编辑（2026-11-13 决定第 (1)(2) 条）：默认只读、头部铅笔钮开关编辑；⌘S 原子写盘（同目录临时文件 + 重命名，保留权限与换行，符号链接写到目标）；Revert；"已打开且脏"的缓冲区在文件切换、Git 写操作、重命名/删除前先问（Save and Continue / Discard Edits / Cancel）；磁盘被别人改写时干净缓冲区静默重载、脏缓冲区出一条 Reload from disk / Keep mine / Compare… 的横条（文件已不在时是 Save As…），不自动处理；自动保存、Compare、Save As 见 files_edit_more.go
 * [POS]: 编辑器自己不跑 git：保存后 Changes 的变化来自快照刷新（saveEdit 只让缓存失效）。未保存的编辑不是 Git 状态，关掉即丢，不进 Safety Journal
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
)

// diskSig is what a file looked like when the buffer last agreed with it.
type diskSig struct {
	mod  time.Time
	size int64
}

func sigOf(path string) (diskSig, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return diskSig{}, false
	}
	return diskSig{mod: info.ModTime(), size: info.Size()}, true
}

// fileEditState is the editing state of the one open file. It belongs to
// path and is dropped with it (claimNativePreview).
type fileEditState struct {
	path     string
	editable bool // a whole, valid UTF-8 text file in the code view
	editing  bool
	sig      diskSig
	// diskChanged: the file was rewritten while the buffer held edits; gone:
	// it was removed or renamed. Both stop Save until the user chooses.
	diskChanged bool
	gone        bool

	// The "unsaved edits" sheet: what is being waited for (cont completes
	// "Save … before you <cont>?") and what runs once the edits are settled.
	askOpen bool
	askCont string
	askThen func()

	// Auto-save: the buffer version the timer was armed for and the timer.
	autoVer   uint64
	autoTimer *time.Timer
	// Compare…: the gutter shows the file on disk (cmpSig is its signature
	// when read) instead of HEAD (headBase is what to put back).
	comparing bool
	cmpSig    diskSig
	headBase  *editor.Buffer
}

// editResetFor drops the edit state when the preview moves to another file.
func (s *Shell) editResetFor(path string) {
	if s.edit.path != path {
		s.edit = fileEditState{path: path}
	}
}

// fileDirty reports whether the open file holds edits that are not on disk.
func (s *Shell) fileDirty() bool {
	st, e := &s.rightPanel, &s.edit
	return e.path != "" && e.path == st.nativePreviewPath && st.nativeCode != nil && st.nativeCode.Buffer != nil &&
		st.nativeCode.Buffer.Modified()
}

// editUnder reports whether target is the open file or a folder holding it.
func (s *Shell) editUnder(target string) bool {
	p := s.edit.path
	return p != "" && target != "" && (p == target || strings.HasPrefix(p, target+string(filepath.Separator)))
}

// canEdit reports whether the open file can be edited right now.
func (s *Shell) canEdit() bool {
	st, e := &s.rightPanel, &s.edit
	return e.editable && e.path != "" && e.path == st.nativePreviewPath && st.nativeCode != nil && st.previewPlanned == e.path
}

// toggleEditing is the pencil button: read-only ⇄ editing. Going back with
// edits not on disk asks first.
func (s *Shell) toggleEditing() {
	if !s.canEdit() {
		return
	}
	if !s.edit.editing {
		s.setEditing(true)
		return
	}
	s.guardDirty("go back to read-only", func() { s.setEditing(false) })
}

func (s *Shell) setEditing(on bool) {
	st, e := &s.rightPanel, &s.edit
	if st.nativeCode == nil {
		return
	}
	e.editing = on
	st.nativeCode.ReadOnly = !on
	st.nativeCode.NoActiveLine = !on // a viewer has no line being worked on
	if on && hasRenderedView(e.path) {
		st.previewSource = true // the formatted view is of the file on disk
	}
}

// saveEdit writes the buffer to disk (⌘S, the Save button) and reports
// whether the file now matches it. Nothing to write counts as saved.
func (s *Shell) saveEdit() bool { return s.save(false) }

// save is saveEdit; quiet (auto-save) leaves out the "Saved" note.
func (s *Shell) save(quiet bool) bool {
	st, e := &s.rightPanel, &s.edit
	if !s.canEdit() || !e.editing || !st.nativeCode.Buffer.Modified() {
		return true
	}
	name := filepath.Base(e.path)
	if e.gone || e.diskChanged {
		s.pendingToast = name + " changed on disk — Reload from disk or Keep mine first."
		return false
	}
	// The file may have been rewritten since the last look: never write over
	// text the user has not seen.
	if sig, ok := sigOf(e.path); !ok {
		e.gone = true
		s.pendingToast = name + " is gone from disk — it was not saved."
		return false
	} else if sig != e.sig {
		e.diskChanged = true
		s.pendingToast = name + " changed on disk — Reload from disk or Keep mine first."
		return false
	}
	if err := writeFileAtomic(e.path, st.currentRoot, st.nativeCode.Buffer.Snapshot()); err != nil {
		slog.Warn("file save failed", "err_class", errClass(err))
		s.pendingToast = "Couldn't save " + name + ": " + err.Error()
		return false
	}
	st.nativeCode.Buffer.MarkSaved()
	s.lspSaved()
	if sig, ok := sigOf(e.path); ok {
		e.sig = sig
	}
	s.refreshDerivedViews()
	s.afterWrite()
	slog.Info("file saved")
	if !quiet {
		s.pendingToast = "Saved " + name
	}
	return true
}

// errClass names an error for the log without its text (a path may be in it).
func errClass(err error) string {
	switch {
	case errors.Is(err, os.ErrPermission):
		return "permission"
	case errors.Is(err, os.ErrNotExist):
		return "missing"
	default:
		return "error"
	}
}

// writeFileAtomic replaces path with snap: a temporary file beside it takes
// the text (and the old permissions) and is renamed over, so the file is
// always either the old text or the new one. A symbolic link is followed to
// its target, which must stay inside root.
func writeFileAtomic(path, root string, snap editor.Snapshot) error {
	return writeAtomic(path, root, func(w io.Writer) error { _, err := snap.WriteTo(w); return err })
}

// writeAtomic is writeFileAtomic for any writer of the new bytes (a rename
// writes the files it changes byte for byte, line endings and all).
func writeAtomic(path, root string, write func(io.Writer) error) error {
	return writeAtomicOpt(path, root, false, write)
}

// writeAtomicOpt is writeAtomic that, with create, may also make the file
// (permissions 0644) — Save As. An empty root allows any place.
func writeAtomicOpt(path, root string, create bool, write func(io.Writer) error) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	} else if create {
		if dir, derr := filepath.EvalSymlinks(filepath.Dir(path)); derr == nil {
			target = filepath.Join(dir, filepath.Base(path))
		}
	}
	if root != "" {
		if canon, err := filepath.EvalSymlinks(root); err == nil {
			rel, err := filepath.Rel(canon, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return errors.New("the file is outside the repository")
			}
		}
	}
	perm := os.FileMode(0o644)
	if info, err := os.Stat(target); err == nil {
		perm = info.Mode().Perm()
	} else if !create || !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".solcogit-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := write(tmp); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// refreshDerivedViews rebuilds what is drawn from the file's text — a
// Markdown file's formatted view and an SVG's drawing — from the buffer, so
// they do not show the file as it was before the save or reload.
func (s *Shell) refreshDerivedViews() {
	st, e := &s.rightPanel, &s.edit
	if st.nativeCode == nil {
		return
	}
	switch {
	case st.nativeMarkdown != nil:
		st.nativeMarkdown = newMarkdownPreview(e.path, st.currentRoot, st.nativeCode.Buffer.String(), s.revealFile)
	case st.nativeSVG != nil:
		if img, err := ui.ParseSVG([]byte(st.nativeCode.Buffer.String())); err == nil {
			st.nativeSVG = img
		}
	}
}

// reloadFromDisk replaces the buffer with what is on disk now (Revert,
// Discard Edits, Reload from disk, a clean buffer after Git rewrote the file)
// and takes the gutter's HEAD version again.
func (s *Shell) reloadFromDisk() {
	st, e := &s.rightPanel, &s.edit
	if st.nativeCode == nil || e.path == "" {
		return
	}
	preview, err := filesview.ReadText(e.path, st.currentRoot)
	if err != nil {
		e.gone = true
		s.pendingToast = filepath.Base(e.path) + " is gone from disk."
		return
	}
	if preview.IsBinary || preview.Truncated || strings.ContainsRune(preview.Content, '\uFFFD') {
		// It is no longer something this editor can hold: show the file
		// again from the top.
		e.editing = false
		path := e.path
		st.selectedFilePath = ""
		s.openFilePreview(path)
		return
	}
	st.nativeCode.Buffer.Reset(preview.Content)
	if sig, ok := sigOf(e.path); ok {
		e.sig = sig
	}
	e.diskChanged, e.gone, e.comparing, e.headBase = false, false, false, nil
	s.refreshDerivedViews()
	s.refreshDiffBase(e.path)
}

// refreshDiffBase reads the file's version in HEAD again (a checkout moves
// it) and hands it to the gutter.
func (s *Shell) refreshDiffBase(path string) {
	if s.git == nil {
		return
	}
	rel, isNew, want := s.diffBasePlan(path)
	if !want {
		return
	}
	runner, gitRoot := s.git.runner, s.git.root
	apply := func(base *editor.Buffer) {
		if st := &s.rightPanel; st.nativeCode != nil && st.nativePreviewPath == path {
			st.nativeCode.DiffBase = base
		}
	}
	if s.win == nil {
		apply(readDiffBase(runner, gitRoot, rel, isNew))
		return
	}
	go func() {
		base := readDiffBase(runner, gitRoot, rel, isNew)
		s.applyOnWindow(func() { apply(base) })
	}()
}

// keepMine accepts that the disk changed and keeps the buffer: the next Save
// writes over the new file.
func (s *Shell) keepMine() {
	e := &s.edit
	if sig, ok := sigOf(e.path); ok {
		e.sig = sig
		e.gone = false
	}
	e.diskChanged = false
	s.endCompare()
}

// syncEditedFile compares the open file with the disk (every snapshot, so
// after any Git operation or an edit by another tool): a clean buffer is
// reloaded silently, a dirty one is only flagged.
func (s *Shell) syncEditedFile() {
	st, e := &s.rightPanel, &s.edit
	if !s.canEdit() {
		return
	}
	sig, ok := sigOf(e.path)
	switch {
	case !ok:
		if s.fileDirty() {
			e.gone = true
		}
	case sig == e.sig:
	case s.fileDirty():
		e.diskChanged = true
		if e.comparing && sig != e.cmpSig {
			s.compareWithDisk()
		}
	default:
		if st.nativeCode != nil {
			s.reloadFromDisk()
		}
	}
}

// guardDirty runs then at once when no open file holds unsaved edits, and
// otherwise asks first. cont finishes the sentence "Save changes before you
// …?".
func (s *Shell) guardDirty(cont string, then func()) {
	if !s.fileDirty() {
		then()
		return
	}
	s.edit.askOpen, s.edit.askCont, s.edit.askThen = true, cont, then
}

// dirtySheet is the sheet guardDirty opens: Save and Continue, Discard Edits,
// Cancel (last, so Enter dismisses safely).
func (s *Shell) dirtySheet(c *ui.Context) {
	e := &s.edit
	if !e.askOpen {
		return
	}
	if !s.fileDirty() { // settled some other way while the sheet was up
		e.askOpen, e.askThen = false, nil
		return
	}
	open := true
	name := filepath.Base(e.path)
	switch ui.AlertDialog(c, &open, "Save changes to “"+name+"”?",
		"You have edits in "+name+" that are not saved. Save them before you "+e.askCont+"?",
		"Save and Continue", "Discard Edits", "Cancel") {
	case 0:
		then := e.askThen
		e.askOpen, e.askThen = false, nil
		if s.saveEdit() && then != nil {
			then()
		}
		return
	case 1:
		then := e.askThen
		e.askOpen, e.askThen = false, nil
		s.reloadFromDisk()
		if then != nil {
			then()
		}
		return
	}
	if !open {
		e.askOpen, e.askThen = false, nil
	}
}

// dirtyFileName is the unsaved file's name, "" when nothing is unsaved.
func (s *Shell) dirtyFileName() string {
	if !s.fileDirty() {
		return ""
	}
	return filepath.Base(s.edit.path)
}

// editControls is the file bar's editing group: the state pill, Revert and
// Save while there are edits, and the pencil that turns editing on or off.
func (s *Shell) editControls(c *ui.Context, selected string) {
	e := &s.edit
	if e.path != selected || !s.canEdit() {
		return
	}
	k := gorexColorsOf(c.Theme().Dark)
	dirty := s.fileDirty()
	switch {
	case !e.editing:
		statusPill(c, "Read-only", ToneMuted)
	case dirty:
		statusPill(c, "Edited", ToneInfo)
	}
	if e.editing && dirty {
		if tipped(c, regularButton(c, "Revert"), "Throw the edits away and show the file on disk").Clicked() {
			s.reloadFromDisk()
		}
		if tipped(c, regularPrimaryButton(c, "Save ⌘S"), "Write the file").Clicked() {
			s.saveRequested()
		}
	}
	label, tip := "Edit this file", "Edit this file"
	if e.editing {
		label, tip = "Back to read-only", "Editing — click to go back to read-only"
	}
	if gorexIconButton(c, k, iconEdit, label, tip, e.editing, gorexPaneBtn, gorexPaneGlyph).Clicked() {
		s.toggleEditing()
	}
}

// editBanner is the bar over the code when the disk moved under the edits.
func (s *Shell) editBanner(c *ui.Context) {
	e := &s.edit
	if !s.canEdit() || (!e.diskChanged && !e.gone) {
		return
	}
	t := c.Theme()
	typ, sp := Typography(), Spacing()
	tokens := designTokens(t.Dark)
	name := filepath.Base(e.path)
	text := name + " changed on disk while you were editing. Your edits are not saved."
	if e.gone {
		text = name + " is gone from disk. Your edits stay here until you leave this file."
	}
	if e.comparing {
		text = "Comparing your edits with " + name + " on disk — the gutter marks where they differ."
	}
	ui.Row(c).FillWidth().Padding(sp.S, sp.XL).Gap(sp.M).AlignItems(ui.Center).
		Background(tokens.StatusBackground(ToneAttention, t.Dark)).Children(func() {
		ui.Text(c, text).FontSize(typ.Body).Grow(1).MinWidth(0)
		if e.gone {
			if regularButton(c, "Save As…").Clicked() {
				s.saveAs()
			}
		} else {
			if regularButton(c, "Reload from disk").Clicked() {
				s.reloadFromDisk()
			}
			if regularButton(c, "Keep mine").Clicked() {
				s.keepMine()
			}
			if e.comparing {
				if regularButton(c, "Done").Clicked() {
					s.endCompare()
				}
			} else if regularButton(c, "Compare…").Clicked() {
				s.compareWithDisk()
			}
		}
	})
}

// UnsavedEdits names the files with edits that are not on disk, one per
// workspace at most (the open file of each).
func (w *Window) UnsavedEdits() []string {
	var names []string
	for _, ws := range w.workspaces {
		if n := ws.sh.dirtyFileName(); n != "" {
			names = append(names, n)
		}
	}
	return names
}

// AskQuit holds a quit that would lose edits: the window comes forward with a
// sheet, and quit runs only when the user chooses Quit Anyway.
func (w *Window) AskQuit(quit func()) {
	w.onUI(func() { w.quitAsk = quit })
}

// quitConfirm is the sheet AskQuit opens (Cancel last: Enter dismisses safely).
func (w *Window) quitConfirm(c *ui.Context) {
	if w.quitAsk == nil {
		return
	}
	names := w.UnsavedEdits()
	if len(names) == 0 {
		w.quitAsk = nil
		return
	}
	open := true
	msg := "Your unsaved edits in “" + strings.Join(names, "”, “") + "” will be lost."
	switch ui.AlertDialog(c, &open, "Quit with unsaved edits?", msg, "Quit Anyway", "Cancel") {
	case 0:
		quit := w.quitAsk
		w.quitAsk = nil
		quit()
		return
	}
	if !open {
		w.quitAsk = nil
	}
}

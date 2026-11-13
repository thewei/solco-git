package nativeui

/**
 * [INPUT]: files_lsp.go 的文档同步与 lsp.Manager 的 Completion / Format / Rename / FilesChanged、editor.State 的 OnCompletion / ShowCompletions / OnInput、files_edit.go 的原子写盘与保存、dialogs.go 的共用文本对话框
 * [OUTPUT]: Files 里的语言服务器写能力（2026-11-13 决定第 (4) 条，E5）：补全（只在编辑模式；需要同时改别处——例如 import——的条目不给，因为编辑器一次只插一处）、格式化（整份文档，缓冲区里的一步撤销；⌥⇧F 或右键菜单）、重命名（F2 或右键菜单；新名字走共用文本对话框；改动落到多个文件：已关闭的文件走同一个原子写盘并原样保留换行，打开的那个走缓冲区、同样是一步撤销、保存仍由用户来；仓库外的文件一个也不写；打开的缓冲区有未保存编辑时先弹表 Save all and rename / Cancel）
 * [POS]: 请求前先把缓冲区此刻的文本交给服务器（补全每敲一下就要最新文本，不能等 250 ms 的静止），答案带版本回来，文本已变就丢。服务器没有该能力就不出现在菜单里，也不发请求。日志从不含路径、文本、名字
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/lsp"
)

// lspRenameJob is a rename the server has answered and that waits on the
// user (unsaved edits) or is being applied.
type lspRenameJob struct {
	files   []lsp.FileEdit
	newName string
	buf     *editor.Buffer
	version uint64 // the buffer version the edits were made for
	open    bool   // the "save first" sheet is up
}

// lspWritable: the open file is in edit mode and its server is running.
func (s *Shell) lspWritable() (lsp.Caps, bool) {
	d := &s.lsp.doc
	if d.path == "" || s.lsp.mgr == nil || !s.canEdit() || !s.edit.editing || d.path != s.edit.path {
		return lsp.Caps{}, false
	}
	if s.lsp.mgr.Status(d.path).State != lsp.StateRunning {
		return lsp.Caps{}, false
	}
	return s.lsp.mgr.Can(d.path), true
}

// lspFlush makes sure the server holds the text the buffer has now.
func (s *Shell) lspFlush() {
	d := &s.lsp.doc
	if d.buf != nil && (!d.sent || d.synced != d.buf.Version()) {
		s.lspPush()
	}
}

// lspKind maps the protocol's CompletionItemKind to an editor icon.
func lspKind(k int) editor.CompletionKind {
	switch k {
	case 2:
		return editor.CompletionMethod
	case 3, 4:
		return editor.CompletionFunction
	case 5, 10:
		return editor.CompletionField
	case 6:
		return editor.CompletionVariable
	case 7, 8, 13, 22, 25:
		return editor.CompletionType
	case 9:
		return editor.CompletionModule
	case 14:
		return editor.CompletionKeyword
	case 15:
		return editor.CompletionSnippet
	case 12, 20, 21:
		return editor.CompletionConstant
	}
	return editor.CompletionText
}

// lspMenuItems adds the write items to the editor's context menu when the
// server can do them; it closes them off with a separator.
func (s *Shell) lspMenuItems(m *ui.Menu) {
	caps, ok := s.lspWritable()
	if !ok || !(caps.Rename || caps.Format) {
		return
	}
	m.Separator()
	if caps.Rename && m.Item("Rename Symbol…").Chosen() {
		s.lspRenameStart()
	}
	if caps.Format && m.Item("Format Document").Chosen() {
		s.lspFormat()
	}
}

// lspInput takes F2 (rename) and ⌥⇧F (format) before the editor sees them.
func (s *Shell) lspInput(ev ui.InputEvent) bool {
	if ev.Kind != ui.InputKeyDown || ev.Repeat {
		return false
	}
	mods := ev.Mods & (ui.Super | ui.Ctrl | ui.Alt | ui.Shift)
	caps, ok := s.lspWritable()
	if !ok {
		return false
	}
	switch {
	case ev.Key == ui.KeyF2 && mods == 0 && caps.Rename:
		s.lspRenameStart()
		return true
	case (ev.Key == ui.KeyF || ev.PhysicalKey == ui.KeyF) && mods == ui.Alt|ui.Shift && caps.Format:
		s.lspFormat()
		return true
	}
	return false
}

// lspComplete asks the server for completions where the caret is and shows
// them if the text has not moved on.
func (s *Shell) lspComplete(code *editor.State, path string, req editor.CompletionRequest) {
	caps, ok := s.lspWritable()
	if !ok || !caps.Completion || s.lsp.doc.path != path {
		return
	}
	s.lspFlush()
	d := &s.lsp.doc
	mgr, buf, ver := s.lsp.mgr, d.buf, d.base+req.Version
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		items, err := mgr.Completion(ctx, path, ver, req.Offset)
		if err != nil || len(items) == 0 {
			return
		}
		s.applyOnUI(func() {
			if s.lsp.doc.buf != buf || buf.Version() != req.Version || s.rightPanel.nativeCode != code {
				return
			}
			start, list := lspCompletions(items)
			code.ShowCompletions(start, list)
		})
	}()
}

// lspCompletions turns the server's items into the editor's. The editor
// replaces one stretch (start to the caret) for the whole list, so the start
// most items agree on is used and the odd one out is left off.
func lspCompletions(items []lsp.CompletionItem) (int, []editor.Completion) {
	votes := map[int]int{}
	for _, it := range items {
		votes[it.Start]++
	}
	start, best := -1, 0
	for st, n := range votes {
		if n > best || (n == best && st > start) {
			start, best = st, n
		}
	}
	out := make([]editor.Completion, 0, len(items))
	for _, it := range items {
		if it.Start != start {
			continue
		}
		c := editor.Completion{Label: it.Label, Detail: it.Detail, Kind: lspKind(it.Kind), Documentation: it.Doc, FilterText: it.Filter}
		if it.Snippet {
			if sn, err := editor.ParseSnippet(it.Insert); err == nil {
				c.Snippet = sn
			} else {
				c.Insert = it.Label // a template the editor cannot expand: the name alone
			}
		} else {
			c.Insert = it.Insert
		}
		out = append(out, c)
	}
	return start, out
}

// lspFormat formats the whole document through the server: one undo step in
// the buffer, nothing written until the user saves.
func (s *Shell) lspFormat() {
	caps, ok := s.lspWritable()
	if !ok || !caps.Format {
		return
	}
	s.lspFlush()
	d := &s.lsp.doc
	mgr, path, buf, version := s.lsp.mgr, d.path, d.buf, d.buf.Version()
	ver := d.base + version
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		edits, err := mgr.Format(ctx, path, ver)
		s.applyOnUI(func() {
			if s.lsp.doc.buf != buf || buf.Version() != version {
				s.pendingToast = "The text changed while formatting — nothing was changed."
				return
			}
			switch {
			case err != nil:
				s.pendingToast = "Format Document: " + lspFailure(err)
			case len(edits) == 0:
				s.pendingToast = "Already formatted."
			default:
				applyServerEdits(buf, edits)
				s.pendingToast = "Formatted."
			}
		})
	}()
}

// applyServerEdits applies a server's edits to the buffer as one undo step.
func applyServerEdits(buf *editor.Buffer, edits []lsp.TextEdit) {
	bes := lsp.ByteEdits(buf.String(), edits)
	es := make([]editor.Edit, len(bes))
	for i, e := range bes {
		es[i] = editor.Edit{Start: e.Start, End: e.End, Text: e.Text}
	}
	buf.Edit(es...)
}

// lspFailure is what to tell the user about a refused or failed request.
func lspFailure(err error) string {
	if m := lsp.Message(err); m != "" {
		return m
	}
	return "the language server did not answer."
}

// lspWordAt is the identifier around byte offset off of buf.
func lspWordAt(buf *editor.Buffer, off int) string {
	pos := buf.Pos(off)
	line := buf.Line(pos.Line)
	col := min(pos.Col, len(line))
	isWord := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	a := col
	for a > 0 {
		r, n := utf8.DecodeLastRuneInString(line[:a])
		if !isWord(r) {
			break
		}
		a -= n
	}
	b := col
	for b < len(line) {
		r, n := utf8.DecodeRuneInString(line[b:])
		if !isWord(r) {
			break
		}
		b += n
	}
	return line[a:b]
}

// lspRenameStart asks for the new name of the symbol at the caret.
func (s *Shell) lspRenameStart() {
	caps, ok := s.lspWritable()
	code := s.rightPanel.nativeCode
	if !ok || !caps.Rename || code == nil {
		return
	}
	s.lsp.renameAt = code.Cursor()
	word := lspWordAt(code.Buffer, s.lsp.renameAt)
	if word == "" {
		s.pendingToast = "Put the caret on a name to rename it."
		return
	}
	s.openTextDialog("lsp-rename", s.lsp.doc.path, "Rename Symbol", "New name for “"+word+"”", word)
	s.dialogAction = "Rename"
}

// lspRename sends the new name to the server (the dialog's submit).
func (s *Shell) lspRename(path, newName string) {
	caps, ok := s.lspWritable()
	d := &s.lsp.doc
	if !ok || !caps.Rename || d.path != path {
		s.pendingToast = "The file changed. Start the rename again."
		return
	}
	s.lspFlush()
	mgr, buf, off, version := s.lsp.mgr, d.buf, s.lsp.renameAt, d.buf.Version()
	ver := d.base + version
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		files, err := mgr.Rename(ctx, path, ver, off, newName)
		s.applyOnUI(func() {
			if s.lsp.doc.buf != buf || buf.Version() != version {
				s.pendingToast = "The text changed while renaming — nothing was changed."
				return
			}
			if err != nil {
				s.pendingToast = "Rename Symbol: " + lspFailure(err)
				return
			}
			s.lspRenamePlan(&lspRenameJob{files: files, newName: newName, buf: buf, version: version})
		})
	}()
}

// samePath reports whether two paths are one file (a server may have resolved
// symbolic links the app has not).
func samePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// lspRenamePlan decides what a rename's answer needs before it is applied.
func (s *Shell) lspRenamePlan(job *lspRenameJob) {
	if len(job.files) == 0 {
		s.pendingToast = "Nothing to rename."
		return
	}
	outside := 0
	openTouched := false
	for _, f := range job.files {
		if _, ok := s.underRoot(f.Path); !ok {
			outside++
		}
		if samePath(f.Path, s.lsp.doc.path) {
			openTouched = true
		}
	}
	if outside > 0 {
		s.pendingToast = "The rename would change " + itoa(outside) + " file(s) outside this repository — nothing was changed."
		return
	}
	if openTouched && s.fileDirty() {
		job.open = true
		s.lsp.rename = job
		return
	}
	s.lspRenameApply(job)
}

// lspRenameApply writes the rename: closed files first through the atomic
// write, byte for byte; the open buffer last, as one undo step. Everything is
// read and computed before the first write, so a file that cannot be read
// stops the rename with nothing changed.
func (s *Shell) lspRenameApply(job *lspRenameJob) {
	d := &s.lsp.doc
	if d.buf != job.buf || job.buf.Version() != job.version {
		s.pendingToast = "The text changed while renaming — nothing was changed."
		return
	}
	type closed struct {
		path string
		text string
	}
	var writes []closed
	var open []lsp.TextEdit
	for _, f := range job.files {
		if samePath(f.Path, d.path) {
			open = f.Edits
			continue
		}
		data, err := os.ReadFile(f.Path)
		if err != nil || !utf8.Valid(data) {
			s.pendingToast = "Couldn't read " + filepath.Base(f.Path) + " — nothing was changed."
			return
		}
		writes = append(writes, closed{f.Path, lsp.ApplyEdits(string(data), f.Edits)})
	}
	var wrote []string
	var failed []string
	for _, w := range writes {
		text := w.text
		if err := writeAtomic(w.path, s.rightPanel.currentRoot, func(out io.Writer) error {
			_, err := out.Write([]byte(text))
			return err
		}); err != nil {
			failed = append(failed, filepath.Base(w.path))
			continue
		}
		wrote = append(wrote, w.path)
	}
	if len(open) > 0 {
		applyServerEdits(job.buf, open)
	}
	if s.lsp.mgr != nil && len(wrote) > 0 {
		s.lsp.mgr.FilesChanged(wrote)
	}
	n := len(wrote)
	if len(open) > 0 {
		n++
	}
	msg := "Renamed to " + job.newName + " in " + itoa(n) + " file" + pluralS(n) + "."
	if len(open) > 0 {
		msg += " Save " + filepath.Base(d.path) + " to keep its changes."
	}
	if len(failed) > 0 {
		msg = "Renamed in " + itoa(n) + " file" + pluralS(n) + "; couldn't write " + strings.Join(failed, ", ") + "."
	}
	s.pendingToast = msg
	s.lsp.rename = nil
}

// lspRenameSheet is the sheet a rename that touches the unsaved file waits
// behind: save everything and rename, or leave it all as it was.
func (s *Shell) lspRenameSheet(c *ui.Context) {
	job := s.lsp.rename
	if job == nil || !job.open {
		return
	}
	if !s.fileDirty() { // settled some other way while the sheet was up
		s.lsp.rename = nil
		return
	}
	open := true
	name := filepath.Base(s.edit.path)
	others := len(job.files) - 1
	what := name
	if others > 0 {
		what += " and " + itoa(others) + " other file" + pluralS(others)
	}
	switch ui.AlertDialog(c, &open, "Save before renaming?",
		"Renaming to “"+job.newName+"” changes "+what+". "+name+" has edits that are not saved.",
		"Save All and Rename", "Cancel") {
	case 0:
		s.lsp.rename = nil
		if s.saveEdit() {
			s.lspRenameApply(job)
		}
		return
	}
	if !open {
		s.lsp.rename = nil
	}
}

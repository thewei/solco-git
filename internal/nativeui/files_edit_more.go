package nativeui

/**
 * [INPUT]: files_edit.go 的编辑状态与写盘、editor.State.DiffBase、filesview.ReadText、mygo.Dialog.Save、gitBusy
 * [OUTPUT]: E1 遗留的三件事（2026-11-13 决定第 (1)(2) 条）：自动保存（设置里的开关，默认关；停手 1 s 后写盘、静默，Git 操作进行中不写、顺延重试，磁盘被改写或文件已不在时不写）；Compare…（磁盘在脏缓冲区下被改写时，横条上的第三个按钮：把磁盘上的文本交给编辑器的 DiffBase，槽沟标出"我的编辑 vs 磁盘"，用编辑器自己的差异命令，不自造 diff 视图；Done 或 Keep mine 还原 HEAD 的槽沟）；Save As…（原生保存面板；仓库内的新位置写好后在 Files 里打开它、旧缓冲区算已存；仓库外只是存一份副本、缓冲区不动；同一路径 = 把被 Git 删掉的文件原样写回）
 * [POS]: 三者都只经 saveEdit 同一条原子写盘路径或编辑器现成的 DiffBase；不起新的 diff 引擎
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
)

// autoSaveDelay is how long typing must stop before auto-save writes.
var autoSaveDelay = time.Second

// autoSaveTick runs once a frame while the code view shows: with Auto-save on
// it (re)starts the quiet-time timer whenever the buffer changed.
func (s *Shell) autoSaveTick() {
	st, e := &s.rightPanel, &s.edit
	if !s.settings.Workbench.AutoSave || !s.canEdit() || !e.editing || st.nativeCode == nil {
		return
	}
	buf := st.nativeCode.Buffer
	v := buf.Version()
	if !buf.Modified() || v == e.autoVer {
		return
	}
	e.autoVer = v
	s.autoSaveArm(buf, v, autoSaveDelay)
}

func (s *Shell) autoSaveArm(buf *editor.Buffer, v uint64, after time.Duration) {
	e := &s.edit
	if e.autoTimer != nil {
		e.autoTimer.Stop()
	}
	path := e.path
	e.autoTimer = time.AfterFunc(after, func() {
		s.applyOnUI(func() { s.autoSaveFire(path, buf, v) })
	})
}

// autoSaveFire writes the buffer if nothing moved since the timer was armed.
func (s *Shell) autoSaveFire(path string, buf *editor.Buffer, v uint64) {
	st, e := &s.rightPanel, &s.edit
	e.autoTimer = nil
	if !s.settings.Workbench.AutoSave || e.path != path || st.nativeCode == nil || st.nativeCode.Buffer != buf ||
		buf.Version() != v || !buf.Modified() || !s.canEdit() || !e.editing || e.gone || e.diskChanged {
		return
	}
	if s.gitBusy() { // never write under a running Git operation: ask again later
		s.autoSaveArm(buf, v, autoSaveDelay)
		return
	}
	s.save(true)
}

// compareWithDisk shows the file as it is on disk against the buffer: the
// editor's gutter marks where the two differ. The buffer is untouched.
func (s *Shell) compareWithDisk() {
	st, e := &s.rightPanel, &s.edit
	if !s.canEdit() || st.nativeCode == nil {
		return
	}
	preview, err := filesview.ReadText(e.path, st.currentRoot)
	if err != nil {
		e.gone = true
		s.pendingToast = filepath.Base(e.path) + " is gone from disk."
		return
	}
	if preview.IsBinary || preview.Truncated || strings.ContainsRune(preview.Content, '\uFFFD') {
		s.pendingToast = filepath.Base(e.path) + " on disk can't be compared here."
		return
	}
	if !e.comparing {
		e.headBase = st.nativeCode.DiffBase
	}
	st.nativeCode.DiffBase = editor.NewBuffer(preview.Content)
	e.comparing = true
	e.cmpSig, _ = sigOf(e.path)
}

// endCompare puts the gutter back on the file's version in HEAD.
func (s *Shell) endCompare() {
	st, e := &s.rightPanel, &s.edit
	if !e.comparing {
		return
	}
	e.comparing = false
	if st.nativeCode != nil {
		st.nativeCode.DiffBase = e.headBase
	}
	e.headBase = nil
}

// saveAs is Save As…: the native panel, then saveAsTo.
func (s *Shell) saveAs() {
	e := &s.edit
	if !s.canEdit() || !e.editing || s.win == nil {
		return
	}
	win, def := s.win, e.path
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: win, Title: "Save As", DefaultPath: def, CreateDirectories: true,
		})
		if err != nil || path == "" {
			return
		}
		s.applyOnWindow(func() { s.saveAsTo(path) })
	}()
}

// saveAsTo writes the buffer to path. Inside the repository the file is then
// the one open (the old buffer counts as saved: its edits live in the new
// file); outside it a copy is written and the buffer stays as it is. The same
// path as the open file writes a file Git removed back.
func (s *Shell) saveAsTo(path string) {
	st, e := &s.rightPanel, &s.edit
	if !s.canEdit() || st.nativeCode == nil {
		return
	}
	path = filepath.Clean(path)
	snap := st.nativeCode.Buffer.Snapshot()
	if err := writeAtomicOpt(path, "", true, func(w io.Writer) error { _, err := snap.WriteTo(w); return err }); err != nil {
		slog.Warn("file save as failed", "err_class", errClass(err))
		s.pendingToast = "Couldn't save " + filepath.Base(path) + ": " + err.Error()
		return
	}
	slog.Info("file saved as")
	switch rel, inside := s.underRoot(path); {
	case samePath(path, e.path):
		st.nativeCode.Buffer.MarkSaved()
		e.gone, e.diskChanged = false, false
		s.endCompare()
		s.lspSaved()
		if sig, ok := sigOf(e.path); ok {
			e.sig = sig
		}
		s.refreshDerivedViews()
		s.afterWrite()
		s.pendingToast = "Saved " + filepath.Base(path)
	case inside:
		st.nativeCode.Buffer.MarkSaved()
		s.afterWrite()
		s.refreshFiles()                        // the Files tree lists the new file
		s.revealFile(filepath.Join(s.dir, rel)) // the panel's path may come by a symlink the root does not name
		s.pendingToast = "Saved as " + filepath.Base(path)
	default:
		s.pendingToast = "Saved a copy as " + filepath.Base(path) + ". " + filepath.Base(e.path) + " is unchanged."
	}
}

// afterWrite lets Changes follow a file written inside the repository.
func (s *Shell) afterWrite() {
	if s.git != nil && s.git.root != "" {
		s.git.cache.Invalidate(s.git.root)
		s.ensureGitSnapshot(true)
	}
}

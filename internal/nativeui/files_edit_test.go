package nativeui

/**
 * [INPUT]: files_edit.go、files_native_preview.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: Files 就地编辑的合同（2026-11-13 决定第 (1)(2) 条）：默认只读且铅笔钮开关；⌘S 原子写盘（权限、CRLF、不留临时文件、失败不动原文件且缓冲区仍脏）；有未保存编辑时切文件 / Git 写操作先问，Cancel 什么都不动、Discard 之后才继续、Save 先写后继续，不碰工作区的操作不问；磁盘被改写：干净缓冲区静默重载、脏缓冲区只标记且 Save 拒绝、Keep mine 之后才能覆盖；截断 / 非文本不可编辑；退出前的未保存确认
 * [POS]: 每条规则都做过变异检查（去掉询问分支 / 去掉原子重命名 / 去掉磁盘签名比较，对应测试必红）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/filesview"
)

// editShell opens file rel of a fresh repository in the Files view.
func editShell(t *testing.T, files map[string][]byte, rel string) (*Window, *uiPump, *Shell, *ui.Tester, string) {
	t.Helper()
	w, pump, sh, tt, root := previewShell(t, files)
	path := filepath.Join(root, rel)
	openPreview(t, sh, pump, tt, path)
	return w, pump, sh, tt, path
}

func typeAtEnd(sh *Shell, text string) {
	b := sh.rightPanel.nativeCode.Buffer
	b.Edit(editor.Edit{Start: b.Len(), End: b.Len(), Text: text})
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFileOpensReadOnlyAndThePencilTurnsEditingOn(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\ntwo\n")}, "a.txt")
	st := &sh.rightPanel
	if !st.nativeCode.ReadOnly || sh.edit.editing || !sh.canEdit() {
		t.Fatalf("a file opens read-only and editable: ro=%v editing=%v canEdit=%v", st.nativeCode.ReadOnly, sh.edit.editing, sh.canEdit())
	}
	if !tt.HasText("Read-only") {
		t.Errorf("the bar says Read-only: %q", tt.Texts())
	}
	if tt.HasText("Save ⌘S") {
		t.Error("no Save while read-only")
	}
	saveEvidence(t, "edit-readonly", tt)
	sh.toggleEditing()
	settle(tt, pump, 2)
	if st.nativeCode.ReadOnly || !sh.edit.editing {
		t.Fatal("the pencil turns editing on")
	}
	if tt.HasText("Read-only") {
		t.Error("Read-only is gone while editing")
	}
	typeAtEnd(sh, "three\n")
	settle(tt, pump, 2)
	for _, want := range []string{"Edited", "Save ⌘S", "Revert"} {
		if !tt.HasText(want) {
			t.Errorf("an edited buffer shows %q: %q", want, tt.Texts())
		}
	}
	if read(t, path) != "one\ntwo\n" {
		t.Fatal("typing never writes the file")
	}
	saveEvidence(t, "edit-edited", tt)
	sh.toggleEditing() // back to read-only with edits: asks
	if !sh.edit.askOpen || !sh.edit.editing {
		t.Fatal("leaving edit mode with unsaved edits asks first")
	}
}

func TestSaveWritesAtomicallyAndKeepsModeAndLineEndings(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"crlf.txt": []byte("one\r\ntwo\r\n")}, "crlf.txt")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	sh.toggleEditing()
	typeAtEnd(sh, "three\n")
	if !sh.saveEdit() {
		t.Fatalf("save failed: %q", sh.pendingToast)
	}
	if got := read(t, path); got != "one\r\ntwo\r\nthree\r\n" {
		t.Fatalf("the file keeps its CRLF endings: %q", got)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
	if sh.rightPanel.nativeCode.Buffer.Modified() || sh.fileDirty() {
		t.Error("a saved buffer is clean")
	}
	left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".solcogit-save-*"))
	if len(left) != 0 {
		t.Errorf("a temporary file was left behind: %v", left)
	}
	if !strings.HasPrefix(sh.pendingToast, "Saved") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
	// The Changes list learns of it from the snapshot, not from the editor.
	pump.until(t, "the snapshot sees crlf.txt", func() bool {
		_, ok := sh.git.snapshot.ByPath("crlf.txt")
		return ok
	})
	settle(tt, pump, 2)
}

func TestSaveFollowsASymlinkAndStaysInsideTheRepository(t *testing.T) {
	_, _, sh, _, path := editShell(t, map[string][]byte{"real.txt": []byte("x\n")}, "real.txt")
	link := filepath.Join(filepath.Dir(path), "link.txt")
	if err := os.Symlink(path, link); err != nil {
		t.Skip("no symlinks")
	}
	snap := editor.NewBuffer("y\n").Snapshot()
	if err := writeFileAtomic(link, sh.rightPanel.currentRoot, snap); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the link itself must stay a link")
	}
	if read(t, path) != "y\n" {
		t.Error("the target takes the text")
	}
	outside := filepath.Join(t.TempDir(), "out.txt")
	os.WriteFile(outside, []byte("o"), 0o644)
	esc := filepath.Join(filepath.Dir(path), "esc.txt")
	if err := os.Symlink(outside, esc); err != nil {
		t.Skip("no symlinks")
	}
	if err := writeFileAtomic(esc, sh.rightPanel.currentRoot, snap); err == nil {
		t.Fatal("a link that leaves the repository must not be written through")
	}
	if read(t, outside) != "o" {
		t.Fatal("the outside file is untouched")
	}
}

func TestAFailedSaveLeavesTheFileAndTheEditsAlone(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	_, _, sh, _, path := editShell(t, map[string][]byte{"sub/a.txt": []byte("one\n")}, "sub/a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o755)
	if sh.saveEdit() {
		t.Fatal("saving into a read-only folder must fail")
	}
	if read(t, path) != "one\n" {
		t.Fatal("the file on disk is untouched")
	}
	if !sh.fileDirty() {
		t.Fatal("the edits must still be there, unsaved")
	}
	if !strings.HasPrefix(sh.pendingToast, "Couldn't save") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
}

func TestUnsavedEditsAreAskedAboutBeforeAGitWrite(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	var ran atomic.Int32
	op := func(ctx context.Context, root string) (string, error) { ran.Add(1); return "", nil }

	// An operation that never writes a tracked file does not ask.
	sh.runGitOp("Fetch", op)
	pump.until(t, "Fetch ran", func() bool { return ran.Load() == 1 })
	if sh.edit.askOpen {
		t.Fatal("Fetch needs no question")
	}

	// One that may rewrite the working tree asks, and runs nothing yet.
	sh.runGitOp("Check Out", op)
	settle(tt, pump, 3)
	if ran.Load() != 1 || !sh.edit.askOpen {
		t.Fatalf("Check Out waits for the answer: ran=%d ask=%v", ran.Load(), sh.edit.askOpen)
	}
	for _, want := range []string{"Save changes to “a.txt”?", "Save and Continue", "Discard Edits", "Cancel"} {
		if !tt.HasText(want) {
			t.Errorf("the sheet shows %q: %q", want, tt.Texts())
		}
	}
	tt.Click("Cancel")
	settle(tt, pump, 3)
	if ran.Load() != 1 || sh.edit.askOpen || !sh.fileDirty() || read(t, path) != "one\n" {
		t.Fatalf("Cancel changes nothing: ran=%d ask=%v dirty=%v disk=%q", ran.Load(), sh.edit.askOpen, sh.fileDirty(), read(t, path))
	}

	// Save and Continue writes first, then runs.
	sh.runGitOp("Check Out", op)
	settle(tt, pump, 3)
	tt.Click("Save and Continue")
	settle(tt, pump, 3)
	pump.until(t, "Check Out ran", func() bool { return ran.Load() == 2 })
	if read(t, path) != "one\ntwo\n" || sh.fileDirty() {
		t.Fatalf("saved before the operation: disk=%q dirty=%v", read(t, path), sh.fileDirty())
	}

	// Discard Edits drops them, then runs.
	typeAtEnd(sh, "three\n")
	sh.runGitOp("Reset", op)
	settle(tt, pump, 3)
	tt.Click("Discard Edits")
	settle(tt, pump, 3)
	pump.until(t, "Reset ran", func() bool { return ran.Load() == 3 })
	if read(t, path) != "one\ntwo\n" || sh.fileDirty() || sh.rightPanel.nativeCode.Buffer.String() != "one\ntwo\n" {
		t.Fatalf("discarded: disk=%q buffer=%q", read(t, path), sh.rightPanel.nativeCode.Buffer.String())
	}
}

func TestOpeningAnotherFileAsksAboutUnsavedEdits(t *testing.T) {
	_, pump, sh, tt, a := editShell(t, map[string][]byte{"a.txt": []byte("one\n"), "b.txt": []byte("bee\n")}, "a.txt")
	b := filepath.Join(filepath.Dir(a), "b.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	sh.openFilePreview(b)
	settle(tt, pump, 3)
	if sh.rightPanel.selectedFilePath != a || !sh.edit.askOpen {
		t.Fatalf("the other file waits for the answer: selected=%q ask=%v", sh.rightPanel.selectedFilePath, sh.edit.askOpen)
	}
	tt.Click("Discard Edits")
	settle(tt, pump, 3)
	pump.until(t, "b.txt open", func() bool { return sh.rightPanel.previewPlanned == b && !sh.rightPanel.nativeTextLoading })
	if read(t, a) != "one\n" {
		t.Error("discarding never writes")
	}
	if sh.edit.path != b || sh.edit.editing || sh.fileDirty() {
		t.Errorf("the new file starts read-only and clean: path=%q editing=%v", sh.edit.path, sh.edit.editing)
	}
}

func TestADiskChangeReloadsACleanBufferAndOnlyFlagsADirtyOne(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	// Clean: silently reloaded, scroll and selection are the editor's own.
	if err := os.WriteFile(path, []byte("one\nfrom git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMTime(t, path)
	sh.syncEditedFile()
	if got := sh.rightPanel.nativeCode.Buffer.String(); got != "one\nfrom git\n" {
		t.Fatalf("a clean buffer follows the disk: %q", got)
	}
	if sh.edit.diskChanged {
		t.Error("nothing to ask about a clean buffer")
	}

	// Dirty: flagged, never overwritten, Save refuses.
	sh.toggleEditing()
	typeAtEnd(sh, "mine\n")
	if err := os.WriteFile(path, []byte("rewritten by git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMTime(t, path)
	sh.syncEditedFile()
	settle(tt, pump, 3)
	if !sh.edit.diskChanged || !sh.fileDirty() {
		t.Fatalf("flag the change, keep the edits: changed=%v dirty=%v", sh.edit.diskChanged, sh.fileDirty())
	}
	for _, want := range []string{"changed on disk", "Reload from disk", "Keep mine"} {
		if !tt.HasText(want) {
			t.Errorf("the bar shows %q: %q", want, tt.Texts())
		}
	}
	if sh.saveEdit() || read(t, path) != "rewritten by git\n" {
		t.Fatal("Save refuses while the disk moved under the edits, and writes nothing")
	}

	// Keep mine accepts the disk's state; the next Save overwrites it.
	tt.Click("Keep mine")
	settle(tt, pump, 3)
	if sh.edit.diskChanged {
		t.Fatal("Keep mine clears the flag")
	}
	if !sh.saveEdit() || read(t, path) != "one\nfrom git\nmine\n" {
		t.Fatalf("after Keep mine the buffer is written: %q", read(t, path))
	}

	// Reload from disk throws the edits away.
	typeAtEnd(sh, "more\n")
	os.WriteFile(path, []byte("third\n"), 0o644)
	bumpMTime(t, path)
	sh.syncEditedFile()
	settle(tt, pump, 3)
	tt.Click("Reload from disk")
	settle(tt, pump, 3)
	if got := sh.rightPanel.nativeCode.Buffer.String(); got != "third\n" || sh.fileDirty() {
		t.Fatalf("Reload from disk shows the file: %q", got)
	}
}

// bumpMTime moves a file's modification time forward: two writes inside the
// filesystem's timestamp grain would otherwise look the same.
func bumpMTime(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	later := info.ModTime().Add(2e9)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestTruncatedAndNonTextFilesAreNotEditable(t *testing.T) {
	big := bytes.Repeat([]byte("line of text\n"), filesview.TextPreviewCap/13+10)
	_, _, sh, tt, _ := editShell(t, map[string][]byte{"big.txt": big, "ok.txt": []byte("ok\n")}, "big.txt")
	if sh.canEdit() {
		t.Fatal("a file cut at the preview cap would be written back short: not editable")
	}
	if tt.HasText("Read-only") {
		t.Error("no Read-only badge or pencil where editing is not offered")
	}
	sh.toggleEditing()
	if sh.edit.editing {
		t.Error("the pencil does nothing on a file that cannot be edited")
	}
}

func TestQuitAsksWhenAFileHasUnsavedEdits(t *testing.T) {
	w, pump, sh, tt, _ := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	if len(w.UnsavedEdits()) != 0 {
		t.Fatal("nothing unsaved yet")
	}
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	if got := w.UnsavedEdits(); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("UnsavedEdits = %v", got)
	}
	quit := 0
	w.AskQuit(func() { quit++ })
	settle(tt, pump, 3)
	if !tt.HasText("Quit with unsaved edits?") || !tt.HasText("a.txt") {
		t.Fatalf("the quit sheet names the file: %q", tt.Texts())
	}
	tt.Click("Cancel")
	settle(tt, pump, 3)
	if quit != 0 || w.quitAsk != nil {
		t.Fatalf("Cancel keeps running: quit=%d", quit)
	}
	w.AskQuit(func() { quit++ })
	settle(tt, pump, 3)
	tt.Click("Quit Anyway")
	settle(tt, pump, 3)
	if quit != 1 {
		t.Fatalf("Quit Anyway quits: %d", quit)
	}
}

func TestRemovingAWorkspaceWithUnsavedEditsAsks(t *testing.T) {
	w, _, sh, _, _ := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	ws := w.workspaces[w.active]
	if w.workspaceNeedsConfirm(ws) {
		t.Fatal("a clean workspace is removed without a question")
	}
	sh.toggleEditing()
	typeAtEnd(sh, "x")
	if !w.workspaceNeedsConfirm(ws) {
		t.Fatal("unsaved edits are lost with the workspace: ask")
	}
}

func TestRenameOrDeleteOfTheEditedFileIsRefused(t *testing.T) {
	_, _, sh, _, path := editShell(t, map[string][]byte{"d/a.txt": []byte("one\n")}, "d/a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	if !sh.editUnder(path) || !sh.editUnder(filepath.Dir(path)) || sh.editUnder(path+"x") {
		t.Fatal("editUnder names the file and the folders holding it")
	}
	sh.runFilesDelete(filepath.Dir(path))
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("a folder holding unsaved edits is not deleted: %v", err)
	}
	if !strings.Contains(sh.pendingToast, "Save or discard") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
}

// A disk change nobody has noticed yet (no snapshot since) must still stop
// Save: it re-reads the file's signature itself.
func TestSaveNoticesADiskChangeNobodySawYet(t *testing.T) {
	_, _, sh, _, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	os.WriteFile(path, []byte("elsewhere\n"), 0o644)
	bumpMTime(t, path)
	if sh.saveEdit() {
		t.Fatal("Save must not write over a file that changed under it")
	}
	if read(t, path) != "elsewhere\n" || !sh.edit.diskChanged || !sh.fileDirty() {
		t.Fatalf("disk=%q changed=%v dirty=%v", read(t, path), sh.edit.diskChanged, sh.fileDirty())
	}
}

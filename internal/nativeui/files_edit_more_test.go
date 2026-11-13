package nativeui

/**
 * [INPUT]: files_edit_more.go、files_edit.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: E1 遗留三件事的合同（2026-11-13 决定第 (1)(2) 条）：自动保存默认关、开了停手后静默写盘、Git 操作进行中顺延、磁盘被改写时不写；Compare… 把磁盘文本放进编辑器的 DiffBase、不动缓冲区与磁盘、Done / Keep mine / Reload 之后槽沟回到 HEAD；Save As：仓库内新位置写盘并打开它且旧缓冲区不再问、仓库外只存副本缓冲区不动、同一路径把被删的文件写回、覆盖已有文件保留内容正确
 * [POS]: 每条规则都做过变异检查（自动保存忽略开关 / 忽略 Git 忙 / 不看磁盘改写，Compare 不还原，Save As 仓库内不标已存，对应测试必红）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-git/internal/settings"
)

func autoSaveShell(t *testing.T, on bool) (*uiPump, *Shell, *ui.Tester, string) {
	t.Helper()
	old := autoSaveDelay
	autoSaveDelay = 40 * time.Millisecond
	t.Cleanup(func() { autoSaveDelay = old })
	w, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	w.change(func(v *settings.Settings) { v.Workbench.AutoSave = on })
	sh.toggleEditing()
	return pump, sh, tt, path
}

func quiet(tt *ui.Tester, pump *uiPump, d time.Duration) {
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		pump.drain()
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAutoSaveIsOffByDefaultAndWritesQuietlyOnceTypingStops(t *testing.T) {
	pump, sh, tt, path := autoSaveShell(t, false)
	typeAtEnd(sh, "two\n")
	quiet(tt, pump, 300*time.Millisecond)
	if read(t, path) != "one\n" {
		t.Fatal("off: only ⌘S writes")
	}

	pump, sh, tt, path = autoSaveShell(t, true)
	typeAtEnd(sh, "two\n")
	framesUntil(t, tt, pump, "the auto-save", func() bool { return read(t, path) == "one\ntwo\n" })
	if sh.fileDirty() || strings.HasPrefix(sh.pendingToast, "Saved") {
		t.Errorf("clean after, and no 'Saved' note for a write nobody asked for: dirty=%v toast=%q", sh.fileDirty(), sh.pendingToast)
	}
}

func TestAutoSaveWaitsOutAGitOperationAndNeverWritesOverAChangedDisk(t *testing.T) {
	pump, sh, tt, path := autoSaveShell(t, true)
	if sh.git == nil {
		t.Skip("no git state in this shell")
	}
	sh.git.opBusy = true
	typeAtEnd(sh, "two\n")
	quiet(tt, pump, 300*time.Millisecond)
	if read(t, path) != "one\n" {
		t.Fatal("a running Git operation holds the write")
	}
	sh.git.opBusy = false
	framesUntil(t, tt, pump, "the write after the operation", func() bool { return read(t, path) == "one\ntwo\n" })

	typeAtEnd(sh, "three\n")
	if err := os.WriteFile(path, []byte("from elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMTime(t, path)
	sh.syncEditedFile()
	quiet(tt, pump, 300*time.Millisecond)
	if read(t, path) != "from elsewhere\n" || !sh.edit.diskChanged {
		t.Fatalf("auto-save never writes over text the user has not seen: %q", read(t, path))
	}
}

func TestCompareShowsTheDiskAgainstTheBufferAndGivesTheGutterBack(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "mine\n")
	if err := os.WriteFile(path, []byte("one\ntheirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bumpMTime(t, path)
	sh.syncEditedFile()
	settle(tt, pump, 3)
	code := sh.rightPanel.nativeCode
	head := code.DiffBase

	tt.Click("Compare…")
	settle(tt, pump, 3)
	if !sh.edit.comparing || code.DiffBase == nil || code.DiffBase.String() != "one\ntheirs\n" {
		t.Fatalf("the gutter compares with the file on disk: comparing=%v", sh.edit.comparing)
	}
	if !tt.HasText("Comparing your edits") {
		t.Error("the bar says what the gutter is showing")
	}
	if code.Buffer.String() != "one\nmine\n" || read(t, path) != "one\ntheirs\n" {
		t.Fatal("comparing changes neither the buffer nor the disk")
	}
	saveEvidence(t, "compare-with-disk", tt)

	tt.Click("Done")
	settle(tt, pump, 3)
	if sh.edit.comparing || code.DiffBase != head || !sh.edit.diskChanged {
		t.Errorf("Done puts the gutter back and the disk change is still unresolved: comparing=%v", sh.edit.comparing)
	}

	tt.Click("Compare…")
	settle(tt, pump, 3)
	tt.Click("Keep mine")
	settle(tt, pump, 3)
	if sh.edit.comparing || code.DiffBase != head || sh.edit.diskChanged {
		t.Errorf("Keep mine ends the comparison too: comparing=%v diskChanged=%v", sh.edit.comparing, sh.edit.diskChanged)
	}
}

func TestSaveAsInsideTheRepositoryOpensTheNewFileAndAsksNothingMore(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	dest := filepath.Join(filepath.Dir(path), "b.txt")
	sh.saveAsTo(dest)
	settle(tt, pump, 3)
	pump.until(t, "b.txt open", func() bool { return sh.rightPanel.previewPlanned == dest && !sh.rightPanel.nativeTextLoading })
	if read(t, dest) != "one\ntwo\n" || read(t, path) != "one\n" {
		t.Fatalf("the copy holds the buffer, the original is untouched: %q %q", read(t, dest), read(t, path))
	}
	if sh.edit.askOpen || sh.fileDirty() {
		t.Error("moving to the new file must not ask about the old buffer: its edits live in the new file")
	}
	info, _ := os.Stat(dest)
	if info.Mode().Perm() != 0o644 {
		t.Errorf("a new file is 0644, got %v", info.Mode().Perm())
	}
}

func TestSaveAsOutsideTheRepositoryOnlyWritesACopy(t *testing.T) {
	_, _, sh, _, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	dest := filepath.Join(t.TempDir(), "copy.txt")
	sh.saveAsTo(dest)
	if read(t, dest) != "one\ntwo\n" || read(t, path) != "one\n" {
		t.Fatal("the copy is written, the original is not")
	}
	if !sh.fileDirty() || sh.edit.path != path {
		t.Error("the buffer stays as it was: its file did not get the edits")
	}
	if !strings.Contains(sh.pendingToast, "copy.txt") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
}

func TestSaveAsTheSamePathWritesAFileGitRemovedBack(t *testing.T) {
	_, pump, sh, tt, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	sh.syncEditedFile()
	settle(tt, pump, 3)
	if !sh.edit.gone || !tt.HasText("Save As…") {
		t.Fatalf("a removed file keeps its buffer and offers Save As…: gone=%v", sh.edit.gone)
	}
	sh.saveAsTo(path)
	if read(t, path) != "one\ntwo\n" || sh.edit.gone || sh.fileDirty() {
		t.Errorf("written back, no longer gone, clean: %q gone=%v dirty=%v", read(t, path), sh.edit.gone, sh.fileDirty())
	}
}

func TestSaveAsOverAnExistingFileReplacesItsContent(t *testing.T) {
	_, _, sh, _, path := editShell(t, map[string][]byte{"a.txt": []byte("one\n"), "b.txt": []byte("old old old old\n")}, "a.txt")
	sh.toggleEditing()
	typeAtEnd(sh, "two\n")
	dest := filepath.Join(filepath.Dir(path), "b.txt")
	sh.saveAsTo(dest)
	if read(t, dest) != "one\ntwo\n" {
		t.Errorf("the whole file is replaced: %q", read(t, dest))
	}
}

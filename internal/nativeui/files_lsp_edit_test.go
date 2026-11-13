package nativeui

/**
 * [INPUT]: files_lsp_edit.go、internal/lsp/lsptest 的迷你服务器（补全 / 格式化 / 重命名）、真实临时仓库、headless ui.Tester
 * [OUTPUT]: E5 的合同（2026-11-13 决定第 (4) 条）：补全显示服务器的条目、需要改别处的条目不出现、文本已变的答案丢掉、只读时不发请求；格式化是缓冲区里的一步撤销、不写盘；重命名改已关闭的文件（原样保留 CRLF）并让打开的缓冲区一步撤销；有未保存编辑时先弹表，取消什么都不动，保存后一并改；会碰仓库外文件的重命名一个也不写；服务器没有能力就不发请求
 * [POS]: 每条规则都做过变异检查（补全不比版本 / 重命名不查仓库外 / 脏缓冲区不弹表 / 已关闭文件按缓冲区换行写，对应测试必红）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/lsp"
)

// editableGo opens a.go in edit mode with its server running.
func editableGo(t *testing.T, files map[string][]byte) (*uiPump, *Shell, *ui.Tester, string, string) {
	t.Helper()
	pump, sh, tt, path, log := lspShell(t, files, "a.go")
	framesUntil(t, tt, pump, "the server", func() bool {
		return sh.lsp.doc.sent && sh.lspManager().Status(sh.lsp.doc.path).State == lsp.StateRunning
	})
	sh.toggleEditing()
	return pump, sh, tt, path, log
}

func calls(log, method string) int { return strings.Count(serverCalls(log), method) }

func TestCompletionShowsTheServersItemsAndLeavesOutOnesThatNeedAnImport(t *testing.T) {
	pump, sh, tt, _, _ := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\nvar x = fmt.Pri\n")})
	code := sh.rightPanel.nativeCode
	off := strings.Index(code.Buffer.String(), "Pri") + 3
	code.Select(off, off)
	code.OnCompletion(editor.CompletionRequest{Offset: off, Prefix: "Pri", Version: code.Buffer.Version()})
	framesUntil(t, tt, pump, "the list", func() bool { return tt.HasText("Println") })
	if !tt.HasText("Printf") {
		t.Error("every plain item is listed")
	}
	if tt.HasText("Unimported") {
		t.Error("an item that also needs an import edit is left out: the editor inserts one place")
	}
	saveEvidence(t, "lsp-completion", tt)
}

func TestACompletionAnswerForTextThatMovedOnIsDropped(t *testing.T) {
	pump, sh, tt, _, log := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\nvar x = fmt.Pri\n")})
	code := sh.rightPanel.nativeCode
	off := strings.Index(code.Buffer.String(), "Pri") + 3
	code.OnCompletion(editor.CompletionRequest{Offset: off, Prefix: "Pri", Version: code.Buffer.Version()})
	typeAtEnd(sh, "// more\n") // the text moves on before the answer is drawn
	framesUntil(t, tt, pump, "the server to answer", func() bool { return calls(log, "textDocument/completion") == 1 })
	settle(tt, pump, 8)
	if tt.HasText("Println") {
		t.Error("a stale answer must not be shown")
	}
}

func TestReadOnlyAsksTheServerForNothing(t *testing.T) {
	pump, sh, tt, _, log := lspShell(t, map[string][]byte{"a.go": []byte("package a\n\nvar x = fmt.Pri\n")}, "a.go")
	framesUntil(t, tt, pump, "running", func() bool { return strings.Contains(serverCalls(log), "didOpen") })
	code := sh.rightPanel.nativeCode
	code.OnCompletion(editor.CompletionRequest{Offset: 20, Prefix: "Pri", Version: code.Buffer.Version()})
	sh.lspFormat()
	sh.lspRenameStart()
	settle(tt, pump, 5)
	if calls(log, "completion")+calls(log, "formatting")+calls(log, "rename") != 0 || sh.dialogOpen {
		t.Fatalf("read-only mode asks nothing: %s", serverCalls(log))
	}
}

func TestFormatIsOneUndoStepAndWritesNothing(t *testing.T) {
	src := "package a\n\n  func Do() {}\n  var x = 1\n"
	pump, sh, tt, path, _ := editableGo(t, map[string][]byte{"a.go": []byte(src)})
	code := sh.rightPanel.nativeCode
	sh.lspFormat()
	want := "package a\n\n\tfunc Do() {}\n\tvar x = 1\n"
	framesUntil(t, tt, pump, "the formatted text", func() bool { return code.Buffer.String() == want })
	if read(t, path) != src {
		t.Fatal("formatting changes the buffer; the user saves")
	}
	code.Buffer.Undo()
	if code.Buffer.String() != src {
		t.Errorf("one undo takes the whole format back: %q", code.Buffer.String())
	}
	sh.lspFormat()
	framesUntil(t, tt, pump, "formatted again", func() bool { return code.Buffer.String() == want })
	sh.lspFormat()
	framesUntil(t, tt, pump, "the note", func() bool { return tt.HasText("Already formatted") })
}

// A format rewrites the text under an open completion list: the list's word
// start and filter belong to the old text, so it is closed.
func TestAFormatClosesAnOpenCompletionList(t *testing.T) {
	pump, sh, tt, _, _ := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\n  var x = fmt.Pri\n")})
	code := sh.rightPanel.nativeCode
	off := strings.Index(code.Buffer.String(), "Pri") + 3
	code.Select(off, off)
	code.OnCompletion(editor.CompletionRequest{Offset: off, Prefix: "Pri", Version: code.Buffer.Version()})
	framesUntil(t, tt, pump, "the list", func() bool { return tt.HasText("Println") })
	sh.lspFormat()
	framesUntil(t, tt, pump, "the formatted text", func() bool { return strings.Contains(code.Buffer.String(), "\tvar x") })
	settle(tt, pump, 3)
	if tt.HasText("Println") {
		t.Error("the completion list is still open over the reformatted text")
	}
}

func renameAtDo(t *testing.T, sh *Shell, path string) {
	t.Helper()
	code := sh.rightPanel.nativeCode
	at := strings.Index(code.Buffer.String(), "Do")
	code.Select(at+1, at+1)
	sh.lspRenameStart()
	if !sh.dialogOpen || sh.dialogValue != "Do" {
		t.Fatalf("the dialog starts with the symbol's name: open=%v value=%q", sh.dialogOpen, sh.dialogValue)
	}
	sh.dialogOpen = false
	sh.submitTextDialog(sh.dialogKind, sh.dialogTarget, "Run")
}

func TestRenameChangesClosedFilesByteForByteAndTheOpenBufferInOneUndoStep(t *testing.T) {
	a := "package a\n\nfunc Do() {}\nvar x = Do()\n"
	other := "package a\r\nvar y = Do()\r\nvar Doer = 1\r\n"
	pump, sh, tt, path, _ := editableGo(t, map[string][]byte{"a.go": []byte(a), "other.go": []byte(other)})
	code := sh.rightPanel.nativeCode
	renameAtDo(t, sh, path)
	otherPath := filepath.Join(filepath.Dir(path), "other.go")
	framesUntil(t, tt, pump, "the rename", func() bool { return strings.Contains(read(t, otherPath), "Run()") })
	if got := read(t, otherPath); got != "package a\r\nvar y = Run()\r\nvar Doer = 1\r\n" {
		t.Errorf("a closed file keeps its CRLF and whole-word matches only: %q", got)
	}
	framesUntil(t, tt, pump, "the buffer", func() bool { return strings.Contains(code.Buffer.String(), "func Run()") })
	if want := "package a\n\nfunc Run() {}\nvar x = Run()\n"; code.Buffer.String() != want {
		t.Errorf("the buffer follows: %q", code.Buffer.String())
	}
	if read(t, path) != a {
		t.Error("the open file is the user's to save")
	}
	code.Buffer.Undo()
	if code.Buffer.String() != a {
		t.Errorf("one undo takes the whole rename out of the buffer: %q", code.Buffer.String())
	}
	framesUntil(t, tt, pump, "the note", func() bool { return tt.HasText("Renamed to Run in 2 files") })
}

func TestRenameAsksFirstWhenTheOpenFileHasUnsavedEdits(t *testing.T) {
	a := "package a\n\nfunc Do() {}\nvar x = Do()\n"
	other := "package a\nvar y = Do()\n"
	pump, sh, tt, path, _ := editableGo(t, map[string][]byte{"a.go": []byte(a), "other.go": []byte(other)})
	otherPath := filepath.Join(filepath.Dir(path), "other.go")
	typeAtEnd(sh, "// note\n")
	renameAtDo(t, sh, path)
	framesUntil(t, tt, pump, "the sheet", func() bool { return tt.HasText("Save All and Rename") })
	saveEvidence(t, "lsp-rename-sheet", tt)
	if read(t, otherPath) != other || read(t, path) != a {
		t.Fatal("nothing is written before the answer")
	}

	tt.Click("Cancel")
	settle(tt, pump, 4)
	if read(t, otherPath) != other || read(t, path) != a || strings.Contains(sh.rightPanel.nativeCode.Buffer.String(), "Run") {
		t.Fatal("Cancel leaves everything as it was")
	}

	renameAtDo(t, sh, path)
	framesUntil(t, tt, pump, "the sheet again", func() bool { return tt.HasText("Save All and Rename") })
	tt.Click("Save All and Rename")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "the rename", func() bool {
		return strings.Contains(read(t, otherPath), "Run()") && strings.Contains(code.Buffer.String(), "func Run()")
	})
	// Save All settled what the user typed; the rename itself waits in the
	// buffer like a format does.
	if got := read(t, path); got != a+"// note\n" {
		t.Errorf("the edits were saved before the rename, under the old name: %q", got)
	}
	if !sh.fileDirty() {
		t.Error("the rename is unsaved in the buffer")
	}
}

func TestARenameThatReachesOutsideTheRepositoryWritesNothing(t *testing.T) {
	a := "package a\n\nfunc Do() {}\n"
	other := "package a\nvar y = Do()\n"
	pump, sh, tt, path, _ := editableGo(t, map[string][]byte{"a.go": []byte(a), "other.go": []byte(other)})
	code := sh.rightPanel.nativeCode
	at := strings.Index(code.Buffer.String(), "Do")
	code.Select(at+1, at+1)
	sh.lspRenameStart()
	sh.dialogOpen = false
	sh.submitTextDialog(sh.dialogKind, sh.dialogTarget, "OUTSIDE")
	framesUntil(t, tt, pump, "the refusal", func() bool { return tt.HasText("outside this repository") })
	if read(t, filepath.Join(filepath.Dir(path), "other.go")) != other || code.Buffer.String() != a {
		t.Error("no file and no buffer changes")
	}
}

func TestAServerWithoutTheCapabilityIsNeverAsked(t *testing.T) {
	t.Setenv("LSP_FAKE_CAPS", "read")
	pump, sh, tt, _, log := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\nfunc Do() {}\nvar x = fmt.Pri\n")})
	code := sh.rightPanel.nativeCode
	at := strings.Index(code.Buffer.String(), "Do")
	code.Select(at+1, at+1)
	code.OnCompletion(editor.CompletionRequest{Offset: at, Prefix: "Do", Version: code.Buffer.Version()})
	sh.lspFormat()
	sh.lspRenameStart()
	if m := sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF2}); m {
		t.Error("F2 is left to the editor when rename is not offered")
	}
	settle(tt, pump, 5)
	if calls(log, "completion")+calls(log, "formatting")+calls(log, "rename") != 0 || sh.dialogOpen {
		t.Fatalf("no capability, no request: %s", serverCalls(log))
	}
}

func TestF2AndOptionShiftFAreTheServersWhenItCanDoThem(t *testing.T) {
	_, sh, _, _, _ := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\nfunc Do() {}\n")})
	code := sh.rightPanel.nativeCode
	at := strings.Index(code.Buffer.String(), "Do")
	code.Select(at+1, at+1)
	if !sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF2}) || !sh.dialogOpen {
		t.Fatal("F2 opens the rename dialog")
	}
	if !sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF, Mods: ui.Alt | ui.Shift}) {
		t.Fatal("⌥⇧F formats")
	}
	if sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF, Mods: ui.Super}) {
		t.Fatal("other chords are the editor's")
	}
}

// macOS types the Option character right after a keyDown the app took; the
// editor must not receive it (the text it would make the format answer stale).
func TestOptionShiftFDropsTheCharacterMacOSTypesAfterIt(t *testing.T) {
	_, sh, _, _, _ := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\nfunc Do() {}\n")})
	if !sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF, Mods: ui.Alt | ui.Shift}) {
		t.Fatal("⌥⇧F is taken")
	}
	if !sh.lspInput(ui.InputEvent{Kind: ui.InputText, Text: "Ï"}) {
		t.Fatal("the Option character that follows is taken too")
	}
	if sh.lspInput(ui.InputEvent{Kind: ui.InputText, Text: "x"}) {
		t.Fatal("only that one character is dropped; later typing is the editor's")
	}
	sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyF, Mods: ui.Alt | ui.Shift})
	sh.lspInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA})
	if sh.lspInput(ui.InputEvent{Kind: ui.InputText, Text: "a"}) {
		t.Fatal("a key after it clears the pending drop")
	}
}

func TestLspWordAtFindsTheIdentifierAroundTheCaret(t *testing.T) {
	b := editor.NewBuffer("var héllo_1 = 2\n")
	for off, want := range map[int]string{4: "héllo_1", 11: "héllo_1", 12: "héllo_1", 5: "héllo_1", 3: "var"} {
		if got := lspWordAt(b, off); got != want {
			t.Errorf("at %d: %q, want %q", off, got, want)
		}
	}
	if got := lspWordAt(b, 14); got != "" {
		t.Errorf("on punctuation and spaces there is no name: %q", got)
	}
}

func formatOnSaveShell(t *testing.T, on bool) (*uiPump, *Shell, *ui.Tester, string, string) {
	t.Helper()
	pump, sh, tt, path, log := editableGo(t, map[string][]byte{"a.go": []byte("package a\n\n  func Do() {}\n")})
	sh.settings.Workbench.FormatOnSave = on
	typeAtEnd(sh, "// x\n")
	return pump, sh, tt, path, log
}

func TestFormatOnSaveFormatsThenWritesAndIsOffByDefault(t *testing.T) {
	pump, sh, tt, path, log := formatOnSaveShell(t, true)
	sh.saveRequested()
	framesUntil(t, tt, pump, "the save", func() bool { return strings.Contains(read(t, path), "\tfunc Do()") })
	if got := read(t, path); got != "package a\n\n\tfunc Do() {}\n// x\n" || sh.fileDirty() {
		t.Errorf("the file holds the formatted text and the buffer is clean: %q dirty=%v", got, sh.fileDirty())
	}
	if calls(log, "formatting") != 1 {
		t.Errorf("one format request: %s", serverCalls(log))
	}

	pump, sh, tt, path, log = formatOnSaveShell(t, false)
	sh.saveRequested()
	settle(tt, pump, 4)
	if got := read(t, path); got != "package a\n\n  func Do() {}\n// x\n" || calls(log, "formatting") != 0 {
		t.Errorf("off: saved as typed, nothing asked: %q %s", got, serverCalls(log))
	}
}

func TestFormatOnSaveStillSavesWhenTheServerCannotFormatOrTheTextMovedOn(t *testing.T) {
	t.Setenv("LSP_FAKE_CAPS", "read")
	_, sh, _, path, _ := formatOnSaveShell(t, true)
	sh.saveRequested()
	if got := read(t, path); got != "package a\n\n  func Do() {}\n// x\n" {
		t.Errorf("no format capability: the save is immediate and unformatted: %q", got)
	}
}

func TestFormatOnSaveSavesWhatIsThereIfTheTextMovedOnDuringTheFormat(t *testing.T) {
	pump, sh, tt, path, _ := formatOnSaveShell(t, true)
	sh.saveRequested()
	typeAtEnd(sh, "// y\n") // before the answer is applied
	framesUntil(t, tt, pump, "the save", func() bool { return strings.Contains(read(t, path), "// y") })
	if got := read(t, path); !strings.Contains(got, "  func Do()") {
		t.Errorf("a stale format is dropped, the text is saved as it is: %q", got)
	}
}

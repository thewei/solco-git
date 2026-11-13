package nativeui

/**
 * [INPUT]: files_lsp.go、internal/lsp 的 Manager、internal/lsp/lsptest 的迷你服务器（测试二进制自己，真实帧化 JSON-RPC）、真实临时仓库、headless ui.Tester
 * [OUTPUT]: E4 的合同（2026-11-13 决定第 (4) 条）：打开有服务器的文件 → 行内诊断与页脚计数；编辑后静止一会儿才把整份文本交给服务器、诊断随之更新；换文件 = 服务器收到 didClose；保存前先交文本再通知 didSave；悬停显示服务器的话、文本又变了就丢掉过期的答案；定义在仓库内就落到 Files 的那一行那一列、仓库外只说一句；没装服务器只说一句且不报错
 * [POS]: 每条规则都做过变异检查（页脚不读诊断 / 保存先通知后交文本 / 悬停不比版本，对应测试必红）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/lsp"
	"github.com/thewei/solco-git/internal/lsp/lsptest"
)

// lspShell is a Files view on a fresh repository whose Go server is this test
// binary (lsptest). It returns the file the server logs every call to.
func lspShell(t *testing.T, files map[string][]byte, rel string) (*uiPump, *Shell, *ui.Tester, string, string) {
	t.Helper()
	log := lsptest.Env(t)
	old := lspDebounce
	lspDebounce = 30 * time.Millisecond
	t.Cleanup(func() { lspDebounce = old })
	_, pump, sh, tt, root := previewShell(t, files)
	sh.lsp.resolve = func(sp lsp.Spec) []string {
		if sp.ID == "go" {
			return []string{os.Args[0]}
		}
		return nil
	}
	t.Cleanup(sh.lspClose)
	path := filepath.Join(root, rel)
	openPreview(t, sh, pump, tt, path)
	return pump, sh, tt, path, log
}

// framesUntil renders frames (a frame is what notices an edit) and runs the
// background results until cond holds.
func framesUntil(t *testing.T, tt *ui.Tester, pump *uiPump, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		pump.drain()
		tt.Frame()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func serverCalls(log string) string {
	b, _ := os.ReadFile(log)
	return strings.Join(strings.Fields(string(b)), " ")
}

func TestOpeningAGoFileShowsItsProblemsInTheTextAndTheFoot(t *testing.T) {
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{"a.go": []byte("package a\n\nvar x = BAD\n")}, "a.go")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "the problem to land", func() bool { return len(code.Diagnostics()) == 1 })
	d := code.Diagnostics()[0]
	if got := code.Buffer.Slice(d.Start, d.End); got != "BAD" || d.Severity != editor.SeverityError || d.Source != "fake" {
		t.Fatalf("the squiggle covers %q (%v, %q), want BAD as an error from the server", got, d.Severity, d.Source)
	}
	if sh.lsp.doc.errors != 1 {
		t.Fatalf("the foot counts %d errors, want 1", sh.lsp.doc.errors)
	}
	saveEvidence(t, "lsp-problem", tt)

	// A click on the count goes to the problem.
	sh.lspJump()
	if sel := code.Selection(); code.Buffer.Slice(min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)) != "BAD" {
		t.Errorf("the jump selects %v", sel)
	}
}

func TestACleanFileSaysNoProblemsAndAFileWithoutAServerStaysQuiet(t *testing.T) {
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{
		"ok.go": []byte("package a\n"), "lib.rs": []byte("fn main() {}\n"), "notes.txt": []byte("hi\n"),
	}, "ok.go")
	framesUntil(t, tt, pump, "No problems", func() bool { return tt.HasText("No problems") })
	root := sh.dir

	openPreview(t, sh, pump, tt, filepath.Join(root, "lib.rs"))
	settle(tt, pump, 3)
	if !tt.HasText("Rust: no language server") {
		t.Errorf("a file whose server is not installed says so in the foot: %q", tt.Texts())
	}
	if sh.pendingToast != "" {
		t.Errorf("a missing server is not an error toast: %q", sh.pendingToast)
	}
	saveEvidence(t, "lsp-missing", tt)

	openPreview(t, sh, pump, tt, filepath.Join(root, "notes.txt"))
	settle(tt, pump, 3)
	if tt.HasText("language server") || tt.HasText("No problems") {
		t.Errorf("plain text has no server line: %q", tt.Texts())
	}
}

func TestEditingSendsTheTextOnceTypingPausesAndTheProblemsFollow(t *testing.T) {
	pump, sh, tt, path, log := lspShell(t, map[string][]byte{"a.go": []byte("package a\n\nvar x = BAD\n")}, "a.go")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "the first problem", func() bool { return len(code.Diagnostics()) == 1 })
	sh.toggleEditing()

	// Fix it: delete BAD. One change reaches the server and the squiggle goes.
	at := strings.Index(code.Buffer.String(), "BAD")
	code.Buffer.Edit(editor.Edit{Start: at, End: at + 3, Text: "1"})
	framesUntil(t, tt, pump, "the squiggle to go", func() bool {
		return len(code.Diagnostics()) == 0 && tt.HasText("No problems")
	})
	if got := serverCalls(log); !strings.Contains(got, "textDocument/didChange") {
		t.Fatalf("the server never saw the edit: %s", got)
	}
	if read(t, path) != "package a\n\nvar x = BAD\n" {
		t.Fatal("the language server never writes the file")
	}
}

func TestSaveHandsOverTheTextBeforeTellingTheServerItWasSaved(t *testing.T) {
	pump, sh, tt, _, log := lspShell(t, map[string][]byte{"a.go": []byte("package a\n")}, "a.go")
	framesUntil(t, tt, pump, "running", func() bool { return strings.Contains(serverCalls(log), "didOpen") })
	sh.toggleEditing()
	typeAtEnd(sh, "// changed\n")
	// No frame has run since the edit: the debounce has not fired. Saving must
	// not leave the server with the old text.
	if !sh.saveEdit() {
		t.Fatalf("save failed: %q", sh.pendingToast)
	}
	framesUntil(t, tt, pump, "didSave", func() bool { return strings.Contains(serverCalls(log), "didSave") })
	got := serverCalls(log)
	if strings.Index(got, "didChange") < 0 || strings.Index(got, "didChange") > strings.Index(got, "didSave") {
		t.Fatalf("the changed text must reach the server before the save notice: %s", got)
	}
}

func TestMovingToAnotherFileClosesTheFirstOnTheServer(t *testing.T) {
	pump, sh, tt, _, log := lspShell(t, map[string][]byte{"a.go": []byte("package a\n"), "b.txt": []byte("hi\n")}, "a.go")
	framesUntil(t, tt, pump, "didOpen", func() bool { return strings.Contains(serverCalls(log), "didOpen") })
	openPreview(t, sh, pump, tt, filepath.Join(sh.dir, "b.txt"))
	framesUntil(t, tt, pump, "didClose", func() bool { return strings.Contains(serverCalls(log), "didClose") })
}

func TestHoverShowsWhatTheServerSays(t *testing.T) {
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{"a.go": []byte("package a\n\nfunc Do() {}\n")}, "a.go")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "the document to be known", func() bool {
		return sh.lsp.doc.sent && sh.lspManager().Status(sh.lsp.doc.path).State == lsp.StateRunning
	})
	off := strings.Index(code.Buffer.String(), "Do")
	code.OnHover(editor.HoverRequest{Offset: off, Version: code.Buffer.Version()})
	framesUntil(t, tt, pump, "the hover", func() bool { return tt.HasText("Does the thing.") })
	saveEvidence(t, "lsp-hover", tt)
	if tt.HasText("```") {
		t.Error("code fences are dropped from the hover text")
	}
}

func TestGoToDefinitionLandsOnTheLineInsideTheRepositoryAndSaysSoOutside(t *testing.T) {
	other := "package a\n// 1\n// 2\n// 3\n\tvalue := 1\n"
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{
		"a.go": []byte("package a\n\nvar x = Do()\nvar y = NONE\nvar z = OUTSIDE\n"), "other.go": []byte(other),
	}, "a.go")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "running", func() bool { return sh.lspManager().Status(sh.lsp.doc.path).State == lsp.StateRunning })
	text := code.Buffer.String()

	// Inside: other.go opens with line 4, columns 2–6 (a tab is one byte) selected.
	code.OnDefinition(strings.Index(text, "Do"))
	otherPath := filepath.Join(sh.dir, "other.go")
	framesUntil(t, tt, pump, "other.go to open", func() bool {
		c := sh.rightPanel.nativeCode
		return sh.rightPanel.selectedFilePath == otherPath && c != nil && c != code && sh.rightPanel.reveal == nil
	})
	c2 := sh.rightPanel.nativeCode
	sel := c2.Selection()
	if got := c2.Buffer.Slice(min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)); got != "alue" || c2.Buffer.Pos(sel.Head).Line != 4 {
		t.Fatalf("selected %q on line %d, want \"alue\" on line 4 (columns 2–6)", got, c2.Buffer.Pos(sel.Head).Line)
	}
	if got := c2.SelectedText(); got != "alue" { // columns 2–6 of "\tvalue := 1"
		t.Fatalf("selected %q, want the range the server named", got)
	}

	// Back in a.go: nothing there, and a definition outside the repository.
	openPreview(t, sh, pump, tt, filepath.Join(sh.dir, "a.go"))
	code = sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "running again", func() bool {
		return sh.lsp.doc.sent && sh.lspManager().Status(sh.lsp.doc.path).State == lsp.StateRunning
	})
	text = code.Buffer.String()
	sh.pendingToast = ""
	code.OnDefinition(strings.Index(text, "NONE"))
	framesUntil(t, tt, pump, "the no-answer toast", func() bool { return tt.HasText("No definition found.") })
	code.OnDefinition(strings.Index(text, "OUTSIDE"))
	framesUntil(t, tt, pump, "the outside toast", func() bool { return tt.HasText("outside this repository: lib/outside.go") })
	if sh.rightPanel.selectedFilePath != filepath.Join(sh.dir, "a.go") {
		t.Error("an outside definition leaves the open file alone")
	}
}

func TestGoToDefinitionWithoutAServerExplainsInsteadOfDoingNothing(t *testing.T) {
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{"ok.go": []byte("package a\n"), "lib.rs": []byte("fn main() {}\n")}, "ok.go")
	openPreview(t, sh, pump, tt, filepath.Join(sh.dir, "lib.rs"))
	framesUntil(t, tt, pump, "the rust file to be the open one", func() bool { return sh.lsp.doc.path == filepath.Join(sh.dir, "lib.rs") })
	sh.rightPanel.nativeCode.OnDefinition(0)
	if !strings.Contains(sh.pendingToast, "no language server is installed") {
		t.Fatalf("toast = %q", sh.pendingToast)
	}
}

func TestAHoverAnswerForTextThatHasMovedOnIsDropped(t *testing.T) {
	pump, sh, tt, _, _ := lspShell(t, map[string][]byte{"a.go": []byte("package a\n\nfunc Do() {}\n")}, "a.go")
	code := sh.rightPanel.nativeCode
	framesUntil(t, tt, pump, "running", func() bool { return sh.lspManager().Status(sh.lsp.doc.path).State == lsp.StateRunning })
	sh.toggleEditing()
	off := strings.Index(code.Buffer.String(), "Do")
	code.OnHover(editor.HoverRequest{Offset: off, Version: code.Buffer.Version()})
	typeAtEnd(sh, "// more\n") // before the answer can be applied
	for i := 0; i < 40; i++ {
		pump.drain()
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	if tt.HasText("Does the thing.") {
		t.Fatal("an answer about the old text must not be shown over the new one")
	}
}

func TestSettingsListsWhichLanguageServersWereFound(t *testing.T) {
	w, pump := newTestWindow(t)
	w.servers.fixed = []lsp.Detected{
		{Spec: lsp.Specs()[0], Command: []string{"/opt/tools/gopls"}},
		{Spec: lsp.Specs()[2]}, // rust: not found
	}
	tt := newTester(w, 1280, 1000, false)
	w.openSettings()
	settle(tt, pump, 3)
	for _, want := range []string{"Language servers", "Go", "/opt/tools/gopls", "Found", "Rust", "Looks for rust-analyzer on your PATH.", "Not installed"} {
		if !tt.HasText(want) {
			t.Errorf("Settings should say %q: %q", want, tt.Texts())
		}
	}
	saveEvidence(t, "lsp-settings", tt)
}

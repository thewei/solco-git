package nativeui

/**
 * [INPUT]: files_git.go、真实临时仓库（有历史、分支、未提交改动、嵌套目录）、headless ui.Tester
 * [OUTPUT]: Files 视图的 Git 层合同：改动标记与含改动目录、Changed 视图只列改动、槽边基准（已跟踪=HEAD 版本，未跟踪=空，未改动=同文）、Plain|Blame 的可用范围与 blame 行（含"未提交"行）、预览页脚的图例
 * [POS]: 2026-11-07「Solco Git」决定里 Files 一节（Changed、M/U 标记、槽边改动条、Plain|Blame、History）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

// filesFixtureShell is a Files view on the fixture repo with one more change
// nested two folders down.
func filesFixtureShell(t *testing.T) (*Window, *uiPump, *Shell, string) {
	t.Helper()
	root := gitFixture(t, "app")
	writeFile(t, filepath.Join(root, "src", "lib", "util.go"),
		"package lib\n\n// Add sums two ints.\n// It never overflows.\nfunc Add(a, b int) int { return a + b }\n")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewFiles)
	pump.until(t, "three changed files", func() bool { return sh.filesChangeMarks().count == 3 })
	return w, pump, sh, root
}

func TestFilesMarksFollowTheSnapshot(t *testing.T) {
	_, _, sh, root := filesFixtureShell(t)
	marks := sh.filesChangeMarks()
	readme := marks.files[filepath.Join(root, "README.md")]
	notes := marks.files[filepath.Join(root, "notes.txt")]
	if readme == nil || gdStatusLetter(readme) != "M" {
		t.Fatalf("README.md mark = %v, want M", readme)
	}
	if notes == nil || gdStatusLetter(notes) != "U" {
		t.Fatalf("notes.txt mark = %v, want U", notes)
	}
	for _, dir := range []string{"src", filepath.Join("src", "lib")} {
		if !marks.dirs[filepath.Join(root, dir)] {
			t.Errorf("%s should be marked as holding a change", dir)
		}
	}
	if marks.dirs[root] {
		t.Error("the root itself carries no dot")
	}
	// The Changed tree lists folders first, then files by name.
	var names []string
	for _, n := range marks.tree {
		names = append(names, n.name)
	}
	if got := strings.Join(names, ","); got != "src,notes.txt,README.md" {
		t.Errorf("Changed tree order = %s, want src,notes.txt,README.md", got)
	}
	if again := sh.filesChangeMarks(); again != marks {
		t.Error("marks are rebuilt only when the snapshot changes")
	}
}

func TestChangedViewListsOnlyChangedFiles(t *testing.T) {
	w, pump, sh, _ := filesFixtureShell(t)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	sh.filesGit.changedOnly = true
	settle(tt, pump, 3)
	for _, want := range []string{"src", "lib", "util.go", "notes.txt", "README.md"} {
		if !tt.HasText(want) {
			t.Errorf("Changed view lacks %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("main.go") {
		t.Error("an unchanged file must not show in the Changed view")
	}
	if !tt.HasText("Changed") || !tt.HasText("All") {
		t.Error("the scope switch is missing")
	}
}

func TestFilesGutterIsDrawnAgainstHead(t *testing.T) {
	_, _, sh, root := filesFixtureShell(t)
	cases := []struct {
		file    string
		wantNil bool
		base    string
	}{
		{"README.md", false, "# demo\n\nA small project.\n"},                         // modified: HEAD's text
		{"notes.txt", false, ""},                                                     // untracked: nothing, every line is new
		{filepath.Join("src", "main.go"), false, "package main\n\nfunc main() {}\n"}, // unchanged: the same text
	}
	for _, tc := range cases {
		sh.rightPanel.selectedFilePath = ""
		sh.openFilePreview(filepath.Join(root, tc.file))
		code := sh.rightPanel.nativeCode
		if code == nil {
			t.Fatalf("%s: no code view (failed=%q)", tc.file, sh.rightPanel.nativeTextFailed)
		}
		if (code.DiffBase == nil) != tc.wantNil {
			t.Fatalf("%s: DiffBase nil = %v, want %v", tc.file, code.DiffBase == nil, tc.wantNil)
		}
		if code.DiffBase != nil {
			if got := code.DiffBase.String(); got != tc.base {
				t.Errorf("%s: base text = %q, want %q", tc.file, got, tc.base)
			}
		}
	}
}

func TestPlainBlameAndHistoryOnlyForTrackedText(t *testing.T) {
	w, pump, sh, root := filesFixtureShell(t)
	tt := newTester(w, 1280, 800, true)
	sh.openFilePreview(filepath.Join(root, "README.md"))
	settle(tt, pump, 3)
	for _, want := range []string{"Plain", "Blame", "History"} {
		if !tt.HasText(want) {
			t.Errorf("a tracked text file needs %q in its header; texts: %q", want, tt.Texts())
		}
	}
	if !sh.canAnnotate(filepath.Join(root, "README.md")) {
		t.Error("README.md is tracked text")
	}
	sh.openFilePreview(filepath.Join(root, "notes.txt"))
	settle(tt, pump, 3)
	if sh.canAnnotate(filepath.Join(root, "notes.txt")) {
		t.Error("an untracked file has no history to annotate")
	}
	if tt.HasText("Blame") || tt.HasText("History") {
		t.Errorf("an untracked file shows no Blame/History; texts: %q", tt.Texts())
	}
}

func TestBlameShowsWhoWroteEachLine(t *testing.T) {
	w, pump, sh, root := filesFixtureShell(t)
	tt := newTester(w, 1280, 800, true)
	path := filepath.Join(root, "README.md")
	sh.openFilePreview(path)
	settle(tt, pump, 2)
	if err := tt.Click("Blame"); err != nil {
		t.Fatalf("click Blame: %v", err)
	}
	pump.until(t, "blame lines", func() bool { return !sh.filesGit.blameLoading && len(sh.filesGit.blameLines) > 0 })
	settle(tt, pump, 3)
	lines := sh.filesGit.blameLines
	// The annotation column names the commit by its short hash; the author and
	// the subject sit in the row's tooltip.
	if len(lines) != 5 {
		t.Fatalf("blame lines = %d, want 5", len(lines))
	}
	if lines[0].Text != "# demo" || lines[0].Hash == uncommittedHash || lines[0].Author != "Test" {
		t.Errorf("first line blame = %+v", lines[0])
	}
	if lines[4].Hash != uncommittedHash {
		t.Errorf("the line the working tree added is not committed yet: %+v", lines[4])
	}
	for _, want := range []string{lines[0].Short, "Not committed yet", "# demo", "Second paragraph."} {
		if !tt.HasText(want) {
			t.Errorf("Blame view lacks %q; texts: %q", want, tt.Texts())
		}
	}
	// Back to Plain: the code view returns.
	if err := tt.Click("Plain"); err != nil {
		t.Fatalf("click Plain: %v", err)
	}
	settle(tt, pump, 3)
	if sh.blameOn(path) {
		t.Error("Plain switches the blame off")
	}
}

func TestPreviewFootNamesLanguageAndLines(t *testing.T) {
	w, pump, sh, root := filesFixtureShell(t)
	tt := newTester(w, 1280, 800, true)
	sh.openFilePreview(filepath.Join(root, "src", "lib", "util.go"))
	settle(tt, pump, 3)
	var foot string
	for _, text := range tt.Texts() {
		if strings.Contains(text, "UTF-8") {
			foot = text
		}
	}
	if !strings.HasPrefix(foot, "Go · UTF-8 · 5 lines") {
		t.Errorf("foot = %q, want Go · UTF-8 · 5 lines", foot)
	}
	for _, want := range []string{"Added", "Modified", "Removed", "since HEAD"} {
		if !tt.HasText(want) {
			t.Errorf("the gutter legend lacks %q", want)
		}
	}
}

func TestBlameParsesWorkingTreeLines(t *testing.T) {
	_, _, sh, root := filesFixtureShell(t)
	lines, err := sh.git.runner.Blame(t.Context(), root, "", "README.md")
	if err != nil {
		t.Fatal(err)
	}
	var fresh int
	for _, l := range lines {
		if l.Hash == uncommittedHash {
			fresh++
		}
	}
	if fresh != 2 {
		t.Errorf("uncommitted lines = %d, want 2 (blank + Second paragraph)", fresh)
	}
	var _ gitworkbench.BlameLine
}

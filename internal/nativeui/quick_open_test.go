package nativeui

/**
 * [INPUT]: quick_open.go、quick_launch.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 快开的合同（2026-11-13 决定第 (3) 条）：Files 标签按子序列排名并在 ↵ 时打开所选文件，空输入列最近打开的，无命中就说无命中；Text 标签搜内容、↵ 落在命中行、不足两个字符不搜；右侧预览显示所选文件；切标签保留输入；没有工作区时 ⌘P 落在 Repos
 * [POS]: 排名由 filesview.MatchFiles 负责，这里只钉"面板把它接对了"；变异检查：把 rows 倒序 / 去掉 ↵ 的打开 / 去掉预览的读取，对应测试必红
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"image/color"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func quickShell(t *testing.T) (*Window, *uiPump, *Shell, *ui.Tester, string) {
	t.Helper()
	w, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"src/server/handler.go": []byte("package server\n\n// needle lives here\nfunc Handle() {}\n"),
		"main/zeta.txt":         []byte("z\n"),
	})
	sh.showView(viewChanges)
	settle(tt, pump, 2)
	return w, pump, sh, tt, root
}

func rowNames(qo *quickOpenState) []string {
	var out []string
	for _, r := range qo.rows {
		out = append(out, r.rel)
	}
	return out
}

func TestQuickFilesRanksAndReturnOpensTheFile(t *testing.T) {
	w, pump, sh, tt, root := quickShell(t)
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("mgo")
	settle(tt, pump, 4)
	rows := rowNames(w.quick.qo)
	if len(rows) == 0 || rows[0] != "src/main.go" {
		t.Fatalf("mgo should put src/main.go first: %q", rows)
	}
	if qo := w.quick.qo; qo.prev == nil || qo.prev.Buffer.String() != "package main\n\nfunc main() {}\n" || !qo.prev.ReadOnly {
		t.Errorf("the preview is the selected file, read-only: %+v", qo.prev)
	}
	saveEvidence(t, "quick-files", tt)
	tt.Key(0, ui.KeyEnter)
	pump.until(t, "the file opens", func() bool {
		return sh.rightPanel.selectedFilePath == filepath.Join(root, "src", "main.go")
	})
	if w.quick.open || sh.currentView() != viewFiles {
		t.Errorf("↵ closes the palette and shows Files: open=%v view=%v", w.quick.open, sh.currentView())
	}
}

func TestQuickFilesRankAFileNameBeforeAFolderName(t *testing.T) {
	w, pump, _, tt, _ := quickShell(t)
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("main")
	settle(tt, pump, 4)
	rows := rowNames(w.quick.qo)
	if len(rows) != 2 || rows[0] != "src/main.go" || rows[1] != "main/zeta.txt" {
		t.Fatalf("a name hit comes before a folder hit: %q", rows)
	}
}

func TestQuickFilesWithoutAQueryListsTheRecentOnesAndNoMatchSaysSo(t *testing.T) {
	w, pump, sh, tt, root := quickShell(t)
	sh.noteRecentFile(filepath.Join(root, "notes.txt"))
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	if rows := rowNames(w.quick.qo); len(rows) != 1 || rows[0] != "notes.txt" {
		t.Fatalf("an empty query lists the recently opened files: %q", rows)
	}
	tt.Type("zzzqq")
	settle(tt, pump, 3)
	if len(w.quick.qo.rows) != 0 || !tt.HasText("No matches") {
		t.Errorf("no match is said, not drawn blank: rows=%q texts=%q", rowNames(w.quick.qo), tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	settle(tt, pump, 2)
	if !w.quick.open {
		t.Error("↵ with no row opens nothing and keeps the palette")
	}
}

func TestQuickTextFindsTheLineAndReturnLandsOnIt(t *testing.T) {
	w, pump, sh, tt, root := quickShell(t)
	w.openQuickTab(quickTabText)
	settle(tt, pump, 3)
	tt.Type("n")
	settle(tt, pump, 3)
	if !tt.HasText("Type at least two characters") {
		t.Errorf("one character does not search: %q", tt.Texts())
	}
	tt.Type("eedle")
	settle(tt, pump, 4)
	rows := w.quick.qo.rows
	if len(rows) != 1 || rows[0].rel != "src/server/handler.go" || rows[0].line != 3 {
		t.Fatalf("the hit is handler.go line 3: %+v", rows)
	}
	saveEvidence(t, "quick-text", tt)
	tt.Key(0, ui.KeyEnter)
	pump.until(t, "the file opens", func() bool {
		return sh.rightPanel.selectedFilePath == filepath.Join(root, "src", "server", "handler.go")
	})
	if w.quick.open {
		t.Error("↵ closes the palette")
	}
}

func TestQuickTabsKeepTheQueryAndWithoutAWorkspaceTheFirstIsRepos(t *testing.T) {
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, false)
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 2)
	if w.quick.tab != quickTabRepos {
		t.Fatalf("with no workspace there is nothing to look into: tab=%d", w.quick.tab)
	}

	w2, pump2, _, tt2, _ := quickShell(t)
	w2.openQuickTab(quickTabFiles)
	settle(tt2, pump2, 2)
	tt2.Type("main")
	settle(tt2, pump2, 2)
	if err := tt2.Click("Text"); err != nil {
		t.Fatal(err)
	}
	settle(tt2, pump2, 3)
	if w2.quick.tab != quickTabText || w2.quick.query != "main" {
		t.Errorf("a tab switch keeps the words: tab=%d query=%q", w2.quick.tab, w2.quick.query)
	}
}

func TestQuickMatchRangesMarkWhatMadeTheRowAHit(t *testing.T) {
	text := func(p string, rs [][2]int) string {
		var out []string
		for _, r := range rs {
			out = append(out, p[r[0]:r[1]])
		}
		return strings.Join(out, "|")
	}
	for _, c := range []struct{ path, query, want string }{
		{"src/main.go", "main", "main"},                   // a name hit
		{"src/main.go", "mgo", "m|go"},                    // a scattered subsequence in the name (g and o touch, so they join)
		{"main/zeta.txt", "main", "main"},                 // a folder hit
		{"src/server/handler.go", "serv han", "serv|han"}, // two words
		{"a/Readme.MD", "readme", "Readme"},               // case is folded, the original text is marked
		{"src/main.go", "ain.g", "ain.g"},                 // a substring with punctuation
	} {
		if got := text(c.path, quickMatchRanges(c.path, c.query)); got != c.want {
			t.Errorf("%q in %q marks %q, want %q", c.query, c.path, got, c.want)
		}
	}
	if got := quickMatchRanges("İstanbul/x.go", "x"); got != nil {
		t.Errorf("a path whose case folding moves offsets is not marked: %v", got)
	}
	if got := quickTextRanges("Needle in a NEEDLE", "needle"); len(got) != 2 || got[0] != [2]int{0, 6} || got[1] != [2]int{12, 18} {
		t.Errorf("every occurrence of a text query: %v", got)
	}
}

func TestQuickRowsCarryTheirMarksAndDrawThem(t *testing.T) {
	w, pump, _, tt, _ := quickShell(t)
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("mgo")
	settle(tt, pump, 4)
	rows := w.quick.qo.rows
	if len(rows) == 0 || len(rows[0].hl) != 2 {
		t.Fatalf("each row knows what matched: %+v", rows)
	}
	if !tt.HasText("main.go") {
		t.Errorf("a marked name is still readable text: %q", tt.Texts())
	}
	saveEvidence(t, "quick-files-marks", tt)

	w.openQuickTab(quickTabText)
	settle(tt, pump, 3)
	tt.Type("needle")
	settle(tt, pump, 4)
	pump.until(t, "the text hit", func() bool { return len(w.quick.qo.rows) > 0 })
	if r := w.quick.qo.rows[0]; len(r.hl) == 0 || !strings.EqualFold(r.text[r.hl[0][0]:r.hl[0][1]], "needle") {
		t.Errorf("a text hit marks the query in its line: %+v", r)
	}
}

func TestQuickCommandReturnLocatesInTheTreeWithoutOpeningAndOptionReturnShowsInFinder(t *testing.T) {
	w, pump, sh, tt, root := quickShell(t)
	zeta := filepath.Join(root, "main", "zeta.txt")
	sh.revealFile(zeta)
	pump.until(t, "zeta open", func() bool { return sh.rightPanel.selectedFilePath == zeta })
	settle(tt, pump, 3)
	sh.filesGit.changedOnly = true // a tree that would hide the row

	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("handler")
	settle(tt, pump, 4)
	handler := filepath.Join(root, "src", "server", "handler.go")
	tt.Key(ui.Super, ui.KeyEnter)
	settle(tt, pump, 4)
	st := &sh.rightPanel
	if w.quick.open || sh.currentView() != viewFiles {
		t.Fatalf("⌘↵ closes the palette and shows Files: open=%v view=%v", w.quick.open, sh.currentView())
	}
	if st.selectedFilePath != zeta {
		t.Errorf("locating opens nothing: the open file is still %q", st.selectedFilePath)
	}
	if st.located != handler || !st.expandedDirs[filepath.Join(root, "src")] || !st.expandedDirs[filepath.Join(root, "src", "server")] {
		t.Errorf("the row is marked and its folders are open: located=%q", st.located)
	}
	if sh.filesGit.changedOnly {
		t.Error("the row must be in the tree: Changed → All")
	}
	if st.locateScroll != "" || !tt.HasText("handler.go") {
		t.Errorf("the row was drawn and scrolled to once: scroll=%q", st.locateScroll)
	}
	saveEvidence(t, "quick-locate", tt)

	sh.openFilePreview(handler)
	if st.located != "" {
		t.Error("opening a file ends the locating")
	}

	var shown string
	old := revealInFinder
	revealInFinder = func(path string) { shown = path }
	t.Cleanup(func() { revealInFinder = old })
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("zeta")
	settle(tt, pump, 4)
	tt.Key(ui.Alt, ui.KeyEnter)
	settle(tt, pump, 3)
	if w.quick.open || shown != zeta || st.selectedFilePath != handler {
		t.Errorf("⌥↵ shows the file in Finder and opens nothing: open=%v shown=%q selected=%q", w.quick.open, shown, st.selectedFilePath)
	}
}

// The located row and the open file are two different things on screen: the
// located one wears the stronger selection face, the open one the softer.
func TestTheLocatedRowReadsApartFromTheOpenFile(t *testing.T) {
	w, pump, sh, tt, root := quickShell(t)
	zeta := filepath.Join(root, "main", "zeta.txt")
	sh.revealFile(zeta)
	pump.until(t, "zeta open", func() bool { return sh.rightPanel.selectedFilePath == zeta })
	settle(tt, pump, 3)
	w.openQuickTab(quickTabFiles)
	settle(tt, pump, 3)
	tt.Type("handler")
	settle(tt, pump, 4)
	tt.Key(ui.Super, ui.KeyEnter)
	settle(tt, pump, 6)

	face := func(name string) color.RGBA {
		t.Helper()
		info, ok := tt.Access(name)
		if !ok {
			t.Fatalf("%s is not drawn", name)
		}
		b := info.Bounds
		return tt.Image().RGBAAt(int(b.X+b.W-4), int(b.Y+b.H/2))
	}
	// the first moment is the find colour, then the row settles
	flash := face("File handler.go")
	sh.rightPanel.locateFlash = ""
	settle(tt, pump, 4)
	open, located := face("File zeta.txt"), face("File handler.go")
	if flash == located {
		t.Errorf("a located row flashes before it settles: %v both times", flash)
	}
	if open == located {
		t.Errorf("the located row and the open file share one face: %v", open)
	}
	saveEvidence(t, "located-face", tt)
}

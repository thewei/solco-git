package nativeui

/**
 * [INPUT]: quick_open.go、quick_launch.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 快开的合同（2026-11-13 决定第 (3) 条）：Files 标签按子序列排名并在 ↵ 时打开所选文件，空输入列最近打开的，无命中就说无命中；Text 标签搜内容、↵ 落在命中行、不足两个字符不搜；右侧预览显示所选文件；切标签保留输入；没有工作区时 ⌘P 落在 Repos
 * [POS]: 排名由 filesview.MatchFiles 负责，这里只钉"面板把它接对了"；变异检查：把 rows 倒序 / 去掉 ↵ 的打开 / 去掉预览的读取，对应测试必红
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
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

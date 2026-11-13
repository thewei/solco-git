package nativeui

/**
 * [INPUT]: git_stash_lines_ui.go、真实临时仓库
 * [OUTPUT]: 行级 stash 界面的合同：跨 hunk 选行后 hunk 行出现 “Stash N Lines…”，点它得到预填说明的对话框，确认后 stash 只含选中的行、工作区只少这些行；文件已有暂存改动时按钮不可用并说明原因；对话框打开后仓库换了就拒绝；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (14) 条的界面防线；内核合同在 solco-gitkit gitworkbench/stash_lines_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

func pickChanged(t *testing.T, sh *Shell, f *gdFile, hunk int) {
	t.Helper()
	for i, l := range f.hunks()[hunk].Lines {
		if l.Kind != gitworkbench.KindContext {
			sh.gdPickLine(f, hunk, i, ui.Cmd)
		}
	}
}

func TestStashLinesFromTheHunkRowStashesOnlyThePickedLines(t *testing.T) {
	root, _ := courierRepo(t, "stash-lines-ui")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three", 25: "twenty-five"}))
	sh, pump, tt := courierShell(t, root, 1)
	f := cfOf(t, sh, "doc.txt")
	if len(f.hunks()) != 2 {
		t.Fatalf("hunks = %d, want 2", len(f.hunks()))
	}
	pickChanged(t, sh, f, 1)
	settle(tt, pump, 2)
	saveEvidence(t, "stash-lines-row", tt)
	if err := tt.Click("Stash 2 Lines…"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if !sh.dialogOpen || sh.dialogKind != "git-stash-lines" || !strings.Contains(sh.dialogValue, "2 lines of doc.txt") {
		t.Fatalf("dialog = open %v kind %q value %q", sh.dialogOpen, sh.dialogKind, sh.dialogValue)
	}
	saveEvidence(t, "stash-lines-dialog", tt)
	if err := tt.Click("Stash"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the stash", func() bool { return !sh.git.opBusy && strings.Contains(sh.pendingToast, "Stashed 2 lines") })
	if got := readFile(t, root+"/doc.txt"); got != courierDoc(map[int]string{3: "three"}) {
		t.Errorf("working tree = %q, want only the first change left", got)
	}
	if shown := gitRun(t, root, "stash", "show", "-p", "stash@{0}"); !strings.Contains(shown, "+twenty-five") || strings.Contains(shown, "+three") {
		t.Errorf("the stash:\n%s", shown)
	}
	if sh.git.gdPick.lines != nil && len(sh.git.gdPick.lines) != 0 {
		t.Error("the pick was kept after stashing")
	}
}

func TestStashLinesIsOfferedOnlyWhereTheKernelCanDoIt(t *testing.T) {
	root, _ := courierRepo(t, "stash-lines-staged")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	gitRun(t, root, "add", "doc.txt")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three", 25: "twenty-five"}))
	writeFile(t, root+"/fresh.txt", "new\n")
	sh, pump, tt := courierShell(t, root, 1)
	f := cfOf(t, sh, "doc.txt")

	// both sides carry changes: the combined view cannot say which lines are meant
	if why := sh.stashLinesWhy(f); !strings.Contains(why, "Only unstaged changes") {
		t.Errorf("combined view: why = %q", why)
	}
	// opened from the Unstaged pane the lines are unstaged, but the file also has staged ones
	f.setView(gdViewUnstaged)
	if why := sh.stashLinesWhy(f); !strings.Contains(why, "also has staged changes") {
		t.Fatalf("unstaged view: why = %q, want the staged reason", why)
	}
	pickChanged(t, sh, f, 0)
	sh.openStashLines(f)
	settle(tt, pump, 1)
	if sh.dialogOpen {
		t.Error("the prompt opened for a file with staged changes")
	}
	// the staged side is not stashed by line, and neither is an untracked file
	f.setView(gdViewStaged)
	if why := sh.stashLinesWhy(f); !strings.Contains(why, "Only unstaged changes") {
		t.Errorf("staged view: why = %q", why)
	}
	if why := sh.stashLinesWhy(cfOf(t, sh, "fresh.txt")); !strings.Contains(why, "Only unstaged changes") {
		t.Errorf("untracked: why = %q", why)
	}
}

func TestStashLinesPromptIsRefusedWhenTheRepositoryChanged(t *testing.T) {
	root, _ := courierRepo(t, "stash-lines-stale")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three", 25: "twenty-five"}))
	sh, pump, tt := courierShell(t, root, 1)
	f := cfOf(t, sh, "doc.txt")
	pickChanged(t, sh, f, 1)
	sh.openStashLines(f)
	settle(tt, pump, 1)
	sh.submitStashLines("/somewhere/else", "x")
	if !strings.Contains(sh.pendingToast, "Repository changed") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
	if out := gitRun(t, root, "stash", "list"); strings.TrimSpace(out) != "" {
		t.Errorf("a stale prompt stashed: %q", out)
	}
}

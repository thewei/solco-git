package nativeui

/**
 * [INPUT]: git_history_ui.go 的多选菜单、git_operations.go 的 reviewCommitVs、真实临时仓库
 * [OUTPUT]: “任意两个提交之间的 diff”的界面合同：⌘ 选两个提交后右键有 Compare the Two Commits，审阅的是两者之间的差异（不是较新提交与其父的差异），标题条写明方向，Back to Commit 退回单个提交
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/fork_more_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTwoChosenCommitsCompareAgainstEachOther(t *testing.T) {
	root := historyRepo(t, "compare-two", 5)
	sh, pump, tt := historyShell(t, root)
	c2, c4 := hashOf(t, root, "c2"), hashOf(t, root, "c4")

	// ⌘-click two rows that are not neighbours, then the batch menu.
	sh.pickCommit(sh.git.commits, c4, 0)
	sh.pickCommit(sh.git.commits, c2, ui.Cmd)
	if got := sh.commitChoiceHashes(sh.git.commits); len(got) != 2 {
		t.Fatalf("chosen = %v", got)
	}
	settle(tt, pump, 2)
	if err := tt.RightClick("c4"); err != nil {
		t.Fatalf("no commit row: %v (%q)", err, tt.Texts())
	}
	if menu := tt.Menu(); len(menu) == 0 || menu[0] != "Compare the Two Commits" {
		t.Fatalf("the two-commit menu should lead with the comparison: %q", menu)
	}
	if err := tt.ChooseMenuItem("Compare the Two Commits"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the pair snapshot", func() bool { return sh.git.commitSnap != nil && sh.git.pairBase == c2 })
	snap := sh.git.commitSnap
	if !strings.HasPrefix(snap.DiffBase, c2) || !strings.HasSuffix(snap.DiffBase, c4) {
		t.Fatalf("DiffBase %q, want c2..c4", snap.DiffBase)
	}
	var files []string
	for _, f := range snap.Files {
		files = append(files, f.Path)
	}
	// c3 and c4 land between the pair; c2 itself and c5 do not.
	if strings.Join(files, " ") != "f3.txt f4.txt" {
		t.Errorf("files = %v, want f3.txt f4.txt", files)
	}
	settle(tt, pump, 3)
	if !tt.HasText("Compared " + shortHash(c2) + " → " + shortHash(c4)) {
		t.Errorf("the header should name the direction: %q", tt.Texts())
	}

	// Back to Commit leaves the pair: the commit alone, only its own file.
	if err := tt.Click("Back to Commit"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the single commit", func() bool {
		return sh.git.pairBase == "" && sh.git.commitSnap != nil && len(sh.git.commitSnap.Files) == 1
	})
	if sh.git.compare {
		t.Error("the single commit is not a comparison")
	}
}

func TestOneChosenCommitHasNoComparison(t *testing.T) {
	root := historyRepo(t, "compare-none", 3)
	sh, pump, tt := historyShell(t, root)
	for _, hashes := range [][]string{{hashOf(t, root, "c2")}, {hashOf(t, root, "c1"), hashOf(t, root, "c2"), hashOf(t, root, "c3")}} {
		sh.clearCommitChoice()
		for i, h := range hashes {
			mods := ui.Cmd
			if i == 0 {
				mods = 0
			}
			sh.pickCommit(sh.git.commits, h, mods)
		}
		settle(tt, pump, 2)
		if len(hashes) == 1 {
			continue // a lone row has the ordinary commit menu, not the batch one
		}
		if err := tt.RightClick("c3"); err != nil {
			t.Fatal(err)
		}
		for _, item := range tt.Menu() {
			if item == "Compare the Two Commits" {
				t.Errorf("%d chosen commits cannot be compared as a pair", len(hashes))
			}
		}
		tt.Key(0, ui.KeyEscape)
	}
}

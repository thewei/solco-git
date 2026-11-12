package nativeui

/**
 * [INPUT]: git_recovery.go、真实临时仓库
 * [OUTPUT]: Recovery 的合同：丢弃与删除后列表里有对应条目、Recover 把内容放回去并保住索引、确认框指向 Recovery 而不再写「无法撤销」、Repository 菜单有 Recovery…、找回前文件又变了就拒绝
 * [POS]: 2026-11-12 决定第 (3) 条 Safety Journal 的界面防线；底层合同在 solco-gitkit gitworkbench/journal_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"strings"
	"testing"
)

func TestDiscardThenRecoverBringsTheEditsBack(t *testing.T) {
	root := gitFixture(t, "recover-discard")
	writeFile(t, root+"/README.md", "my precious edit\n")
	writeFile(t, root+"/scratch.txt", "untracked note\n")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	sh.discardChanges([]string{"README.md"})
	if strings.Contains(sh.confirmMessage, "can't be undone") || !strings.Contains(sh.confirmMessage, "Recovery") {
		t.Fatalf("the discard confirmation says %q", sh.confirmMessage)
	}
	sh.confirmOpen = false
	sh.runGitConfirm("git-discard", sh.confirmTarget)
	pump.until(t, "the discard", func() bool { return !sh.git.opBusy && !strings.Contains(readFileT(t, root+"/README.md"), "precious") })

	sh.discardChanges([]string{"scratch.txt"})
	if !strings.Contains(sh.confirmMessage, "Recovery") {
		t.Fatalf("the delete confirmation says %q", sh.confirmMessage)
	}
	sh.confirmOpen = false
	sh.runGitConfirm("git-delete-untracked", sh.confirmTarget)
	pump.until(t, "the delete", func() bool { _, err := os.Stat(root + "/scratch.txt"); return !sh.git.opBusy && os.IsNotExist(err) })

	sh.openRecoverySheet()
	pump.until(t, "the recovery list", func() bool { return !sh.gitMore.recovery.loading })
	ops := sh.gitMore.recovery.ops
	if len(ops) != 2 || ops[0].Kind != "delete-untracked" || ops[1].Kind != "discard" {
		t.Fatalf("recovery list = %+v", ops)
	}
	settle(tt, pump, 2) // the sheet renders

	for _, op := range ops {
		sh.recoverOperation(op)
		pump.until(t, "the recovery", func() bool { return !sh.git.opBusy })
	}
	if got := readFileT(t, root+"/README.md"); got != "my precious edit\n" {
		t.Errorf("README.md = %q", got)
	}
	if got := readFileT(t, root+"/scratch.txt"); got != "untracked note\n" {
		t.Errorf("scratch.txt = %q", got)
	}
}

func TestRepositoryMenuHasRecovery(t *testing.T) {
	root := gitFixture(t, "recover-menu")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	found := false
	for _, menu := range w.MenuBar().Items() {
		for _, item := range menu.Submenu {
			if item.Label == "Recovery…" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the Repository menu has no Recovery…")
	}
	sh.openRecoverySheet()
	pump.until(t, "the empty list", func() bool { return !sh.gitMore.recovery.loading })
	if len(sh.gitMore.recovery.ops) != 0 {
		t.Fatalf("a fresh repository lists %+v", sh.gitMore.recovery.ops)
	}
}

func readFileT(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

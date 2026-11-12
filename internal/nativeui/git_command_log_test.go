package nativeui

/**
 * [INPUT]: git_command_log.go、真实临时仓库、共享命令日志
 * [OUTPUT]: 命令日志的合同：做过的写命令出现在表里且默认不含读、勾选 Show reads 后读也出现、Copy All 按旧到新给出可当脚本用的文本、Clear 只清本仓库、Repository 菜单有 Command Log…、日志里没有 commit 信息；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (11) 条命令日志的界面防线；底层合同在 solco-gitkit gitworkbench/command_log_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

func TestCommandLogListsWhatWasRunAndCopiesIt(t *testing.T) {
	root := gitFixture(t, "command-log")
	writeFile(t, root+"/README.md", "edited\n")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	sh.discardChanges([]string{"README.md"})
	sh.confirmOpen = false
	sh.runGitConfirm("git-discard", sh.confirmTarget)
	pump.until(t, "the discard", func() bool {
		return !sh.git.opBusy && !strings.Contains(readFileT(t, root+"/README.md"), "edited")
	})

	writes := sharedCommandLog().Entries(sh.git.root, false)
	if len(writes) == 0 {
		t.Fatal("the discard left nothing in the command log")
	}
	for _, e := range writes {
		if e.Class == gitworkbench.OpRead {
			t.Fatalf("reads are hidden by default, got %s", e.Line())
		}
	}
	all := sharedCommandLog().Entries(sh.git.root, true)
	if len(all) <= len(writes) {
		t.Fatalf("loading the repository ran reads, but all=%d writes=%d", len(all), len(writes))
	}

	sh.openCommandLog()
	settle(tt, pump, 2)
	saveEvidence(t, "command-log-writes", tt)

	if err := tt.Click("Show reads"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if !sh.gitMore.commands.reads {
		t.Fatal("Show reads did not switch on")
	}
	saveEvidence(t, "command-log-reads", tt)

	var copied string
	old := writeClipboard
	writeClipboard = func(text string) error { copied = text; return nil }
	defer func() { writeClipboard = old }()
	if err := tt.Click("Copy All"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(copied), "\n")
	now := sharedCommandLog().Entries(sh.git.root, true) // background reads may have landed since `all`
	if len(lines) < len(all) || len(lines) > len(now) || !strings.HasPrefix(lines[0], "git ") {
		t.Fatalf("Copy All gave %d lines for %d..%d entries (one line per command): %q", len(lines), len(all), len(now), copied)
	}
	if lines[0] != now[len(now)-1].Line() {
		t.Errorf("Copy All must run oldest to newest; first line %q, oldest command %q", lines[0], now[len(now)-1].Line())
	}
}

func TestCommandLogClearOnlyForgetsThisRepository(t *testing.T) {
	root := gitFixture(t, "command-log-clear")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	other := t.TempDir() + "/not-this-repo"
	sharedCommandLog().Record(gitworkbench.CommandEntry{Root: other, Args: []string{"add", "x"}, Class: gitworkbench.OpMutate})
	sharedCommandLog().Record(gitworkbench.CommandEntry{Root: sh.git.root, Args: []string{"add", "y"}, Class: gitworkbench.OpMutate})

	sh.openCommandLog()
	settle(tt, pump, 2)
	if err := tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	for _, e := range sharedCommandLog().Entries(sh.git.root, true) { // a refresh may log a new read meanwhile
		if e.Args[0] == "add" {
			t.Errorf("Clear left %s", e.Line())
		}
	}
	if got := sharedCommandLog().Entries(other, true); len(got) != 1 {
		t.Errorf("Clear touched another repository: %d entries", len(got))
	}
}

func TestRepositoryMenuHasCommandLog(t *testing.T) {
	root := gitFixture(t, "command-log-menu")
	w, pump := newTestWindow(t)
	openAndLoad(t, w, pump, root)
	for _, menu := range w.MenuBar().Items() {
		for _, item := range menu.Submenu {
			if item.Label == "Command Log…" {
				return
			}
		}
	}
	t.Fatal("the Repository menu has no Command Log…")
}

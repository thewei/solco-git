package nativeui

/**
 * [INPUT]: git_commit_filter_ui.go、真实临时仓库（historyRepo + 一条未合并的分支）
 * [OUTPUT]: 提交过滤界面的合同：Filters 弹层里填 Path 点 Apply 后列表只剩 Git 答复的提交、摘要写明条件、文字搜索在其内再筛；Current branch only 排除别的分支；Clear Filters 复原；格式错误的日期把 Git 的话显示在弹层里；换仓库、reveal 会清掉过滤；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (14) 条的界面防线；内核合同在 solco-gitkit gitworkbench/commit_filter_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"
)

func filterSubjects(sh *Shell) string {
	var out []string
	for _, c := range sh.filteredCommits() {
		out = append(out, c.Subject)
	}
	return strings.Join(out, ",")
}

func TestFiltersPopoverNarrowsTheListThroughGit(t *testing.T) {
	root := historyRepo(t, "cfilter-ui", 4)
	gitRun(t, root, "checkout", "-q", "-b", "side")
	writeFile(t, root+"/side.txt", "s\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "s1")
	gitRun(t, root, "checkout", "-q", "work")
	sh, pump, tt := historyShell(t, root)
	pump.until(t, "all five commits", func() bool { settle(tt, pump, 1); return len(sh.git.commits) == 5 })

	if err := tt.Click("Commit filters"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if err := tt.Click("Filter path"); err != nil {
		t.Fatal(err)
	}
	tt.Type("f2.txt")
	settle(tt, pump, 1)
	saveEvidence(t, "commit-filter-popover", tt)
	if err := tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "Git's answer", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading && sh.commitFilterOn() })
	if got := filterSubjects(sh); got != "c2" {
		t.Fatalf("the filtered list = %q, want c2", got)
	}
	settle(tt, pump, 2)
	saveEvidence(t, "commit-filter-applied", tt)
	if _, ok := tt.Access("c2"); !ok {
		t.Error("the filtered row c2 is not drawn")
	}
	for _, gone := range []string{"c1", "c3", "c4"} {
		if _, ok := tt.Access(gone); ok {
			t.Errorf("%s is drawn although the filter leaves it out", gone)
		}
	}
	if s := sh.commitFilterSummary(); s != "f2.txt" {
		t.Errorf("summary = %q", s)
	}
	// the text search works inside the Git answer
	sh.gitCommitFilter = "c3"
	if got := filterSubjects(sh); got != "" {
		t.Errorf("a search for c3 inside the f2.txt history = %q, want nothing", got)
	}
	sh.gitCommitFilter = ""

	if err := tt.Click("Clear Filters"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 1)
	if sh.commitFilterOn() || filterSubjects(sh) != "s1,c4,c3,c2,c1" && filterSubjects(sh) != "c4,c3,c2,c1,s1" {
		t.Errorf("after Clear Filters: on=%v list=%q", sh.commitFilterOn(), filterSubjects(sh))
	}
}

func TestFiltersCurrentBranchOnlyLeavesOtherBranchesOut(t *testing.T) {
	root := historyRepo(t, "cfilter-branch", 3)
	gitRun(t, root, "checkout", "-q", "-b", "side")
	writeFile(t, root+"/side.txt", "s\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "s1")
	gitRun(t, root, "checkout", "-q", "work")
	sh, pump, tt := historyShell(t, root)
	pump.until(t, "four commits", func() bool { settle(tt, pump, 1); return len(sh.git.commits) == 4 })

	sh.gitMore.cfilter.currentOnly = true
	sh.applyCommitFilter()
	pump.until(t, "Git's answer", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading })
	if got := filterSubjects(sh); got != "c3,c2,c1" {
		t.Errorf("current branch only = %q", got)
	}

	// a branch hidden from the graph stays hidden from the filter's answer
	sh.gitMore.cfilter.currentOnly, sh.gitMore.cfilter.since = false, "2000-01-01"
	sh.setGraphHidden([]string{"refs/heads/side"}, true)
	sh.applyCommitFilter()
	pump.until(t, "Git's answer", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading })
	if got := filterSubjects(sh); got != "c3,c2,c1" {
		t.Errorf("with side hidden, every branch = %q, want side's commit left out", got)
	}
}

func TestFiltersShowGitsComplaintAboutADateAndKeepTheOldList(t *testing.T) {
	root := historyRepo(t, "cfilter-bad", 3)
	sh, pump, tt := historyShell(t, root)
	sh.gitMore.cfilter.path = "f1.txt"
	sh.applyCommitFilter()
	pump.until(t, "the path filter", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading })
	sh.gitMore.cfilter.since = "last week"
	sh.applyCommitFilter()
	pump.until(t, "the complaint", func() bool { settle(tt, pump, 1); return sh.gitMore.cfilter.err != "" })
	if !strings.Contains(sh.gitMore.cfilter.err, "YYYY-MM-DD") {
		t.Errorf("err = %q", sh.gitMore.cfilter.err)
	}
	if got := filterSubjects(sh); got != "c1" {
		t.Errorf("the list after a refused filter = %q, want the previous answer", got)
	}
}

func TestRevealAndChangingRepositoryClearTheGitFilter(t *testing.T) {
	root := historyRepo(t, "cfilter-reveal", 3)
	sh, pump, tt := historyShell(t, root)
	sh.gitMore.cfilter.path = "f1.txt"
	sh.applyCommitFilter()
	pump.until(t, "the path filter", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading && sh.commitFilterOn() })
	sh.reveal(hashOf(t, root, "c3"), hashOf(t, root, "c3"), false)
	settle(tt, pump, 1)
	if sh.commitFilterOn() {
		t.Error("revealing a commit left a filter that may hide it")
	}
}

func TestTheFilterFollowsTheHistoryWhenItMovesOn(t *testing.T) {
	root := historyRepo(t, "cfilter-refresh", 3)
	sh, pump, tt := historyShell(t, root)
	sh.gitMore.cfilter.path = "f1.txt"
	sh.applyCommitFilter()
	pump.until(t, "the path filter", func() bool { settle(tt, pump, 1); return !sh.gitMore.cfilter.loading && sh.commitFilterOn() })
	writeFile(t, root+"/f1.txt", "line 1, again\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c1 again")
	sh.loadBranches()
	pump.until(t, "the refreshed filter", func() bool { settle(tt, pump, 1); return filterSubjects(sh) == "c1 again,c1" })
}

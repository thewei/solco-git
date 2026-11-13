package nativeui

/**
 * [INPUT]: git_history_ui.go、真实临时仓库（带远端的、带分支的）
 * [OUTPUT]: 历史改写界面的合同：先预览再改写（Drop / Squash / Fixup / Move / Reword / Amend 到旧提交 / 多个 Cherry-pick）、取消不动仓库、预览拒绝时给出原因、已在远端的提交有警告、冲突的改写被中止且仓库原样、整次改写可由 Undo 退回、⌘ / ⇧ 多选的规则；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (12) 条的界面防线；内核合同在 solco-gitkit gitworkbench/history_preview_test.go 与 engine/history_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// historyRepo makes a clean repository of commits c1 … cN (c1 the oldest),
// each adding its own file, so any run of them can be rewritten.
func historyRepo(t *testing.T, name string, n int) string {
	t.Helper()
	root := gitFixture(t, name)
	emptyBranch(t, root, "work")
	for i := 1; i <= n; i++ {
		writeFile(t, root+"/f"+itoa(i)+".txt", "line "+itoa(i)+"\n")
		gitRun(t, root, "add", ".")
		gitRun(t, root, "commit", "-q", "-m", "c"+itoa(i))
	}
	return root
}

// emptyBranch leaves the fixture's history behind: a new unborn branch, the
// working tree emptied and the other branches deleted, so the commit list holds
// only what a test adds.
func emptyBranch(t *testing.T, root, name string) {
	t.Helper()
	gitRun(t, root, "checkout", "-q", "--orphan", name)
	gitRun(t, root, "rm", "-rfq", ".")
	gitRun(t, root, "clean", "-fdq")
	for _, b := range strings.Fields(gitRun(t, root, "for-each-ref", "--format=%(refname:short)", "refs/heads/")) {
		if b != name {
			gitRun(t, root, "branch", "-D", b)
		}
	}
}

func subjects(t *testing.T, root string) string {
	t.Helper()
	return strings.Join(strings.Fields(gitRun(t, root, "log", "--format=%s")), " ")
}

func hashOf(t *testing.T, root, subject string) string {
	t.Helper()
	return strings.TrimSpace(gitRun(t, root, "log", "--all", "--format=%H", "--grep=^"+subject+"$"))
}

// historyShell opens root with its commit list loaded.
func historyShell(t *testing.T, root string) (*Shell, *uiPump, *ui.Tester) {
	t.Helper()
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	sh.showGitCommits()
	pump.until(t, "the commit list", func() bool { return len(sh.git.commits) > 0 })
	settle(tt, pump, 3)
	return sh, pump, tt
}

func TestDropShowsThePreviewFirstAndThenRewrites(t *testing.T) {
	root := historyRepo(t, "hist-drop", 6)
	sh, pump, tt := historyShell(t, root)
	before := subjects(t, root)

	sh.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: []string{hashOf(t, root, "c3"), hashOf(t, root, "c4")}})
	pump.until(t, "the preview", func() bool { return sh.gitMore.history.open })
	h := sh.gitMore.history
	var struck []string
	for _, l := range h.lines {
		if l.struck {
			struck = append(struck, strings.Fields(l.text)[1])
		}
	}
	if strings.Join(struck, " ") != "c4 c3" || h.above != 1 {
		t.Fatalf("preview struck %v above %d, want c4 c3 and one newer commit", struck, h.above)
	}
	settle(tt, pump, 2)
	saveEvidence(t, "history-drop-preview", tt)
	if got := subjects(t, root); got != before {
		t.Fatalf("the preview changed the repository: %q", got)
	}

	if err := tt.Click("Drop"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the rewrite", func() bool { return !sh.git.opBusy && subjects(t, root) != before })
	if got := subjects(t, root); got != "c6 c5 c2 c1" {
		t.Fatalf("log = %q, want c3 and c4 gone", got)
	}

	// the hint in the sheet is true: Undo Git Action puts the branch back
	plan, ok, err := sh.git.runner.PlanReplay(context.Background(), root, false)
	if err != nil || !ok {
		t.Fatalf("nothing to undo after the rewrite: ok=%v err=%v", ok, err)
	}
	if err := sh.git.runner.Replay(context.Background(), root, false, plan.Identity); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, root); got != before {
		t.Errorf("after Undo log = %q, want %q", got, before)
	}
}

func TestCancellingThePreviewLeavesTheRepositoryAlone(t *testing.T) {
	root := historyRepo(t, "hist-cancel", 4)
	sh, pump, tt := historyShell(t, root)
	before := gitRun(t, root, "rev-parse", "HEAD")
	sh.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: []string{hashOf(t, root, "c2")}})
	pump.until(t, "the preview", func() bool { return sh.gitMore.history.open })
	settle(tt, pump, 2)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if sh.gitMore.history.open || gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Error("Cancel must close the sheet and change nothing")
	}
}

func TestSquashFixupMoveAndRewordRewriteTheChosenCommits(t *testing.T) {
	cases := []struct {
		name string
		op   func(t *testing.T, root string) gitworkbench.HistoryOp
		want string
	}{
		{"squash", func(t *testing.T, root string) gitworkbench.HistoryOp {
			return gitworkbench.HistoryOp{Action: gitworkbench.HistorySquash, Hashes: []string{hashOf(t, root, "c3"), hashOf(t, root, "c4")}}
		}, "c6 c5 c2 c1"},
		{"fixup", func(t *testing.T, root string) gitworkbench.HistoryOp {
			return gitworkbench.HistoryOp{Action: gitworkbench.HistoryFixup, Hashes: []string{hashOf(t, root, "c4")}}
		}, "c6 c5 c3 c2 c1"},
		{"move down", func(t *testing.T, root string) gitworkbench.HistoryOp {
			return gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: []string{hashOf(t, root, "c4")}, Offset: 1}
		}, "c6 c5 c3 c4 c2 c1"},
		{"move up", func(t *testing.T, root string) gitworkbench.HistoryOp {
			return gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: []string{hashOf(t, root, "c3")}, Offset: -1}
		}, "c6 c5 c3 c4 c2 c1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := historyRepo(t, "hist-"+strings.ReplaceAll(tc.name, " ", "-"), 6)
			sh, pump, tt := historyShell(t, root)
			before := subjects(t, root)
			sh.openHistoryRewrite(tc.op(t, root))
			pump.until(t, "the preview", func() bool { return sh.gitMore.history.open })
			settle(tt, pump, 2)
			saveEvidence(t, "history-"+strings.ReplaceAll(tc.name, " ", "-")+"-preview", tt)
			run := sh.gitMore.history.run
			sh.gitMore.history.open = false
			sh.runGitOp("test", func(ctx context.Context, root string) (string, error) { return run(ctx, root, "") })
			pump.until(t, "the rewrite", func() bool { return !sh.git.opBusy && subjects(t, root) != before })
			if got := subjects(t, root); got != tc.want {
				t.Errorf("log = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRewordTakesTheNewMessage(t *testing.T) {
	root := historyRepo(t, "hist-reword", 4)
	sh, pump, tt := historyShell(t, root)
	var c3 gitworkbench.CommitInfo
	for _, c := range sh.git.commits {
		if c.Subject == "c3" {
			c3 = c
		}
	}
	sh.openReword(c3)
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	if sh.gitMore.history.msg != "c3" {
		t.Fatalf("the field starts at %q, want the commit's own message", sh.gitMore.history.msg)
	}
	sh.gitMore.history.msg = "c3 but better\n\nwith a body"
	settle(tt, pump, 2)
	saveEvidence(t, "history-reword", tt)
	if err := tt.Click("Reword"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the rewrite", func() bool { return !sh.git.opBusy && strings.Contains(subjects(t, root), "better") })
	if got := subjects(t, root); got != "c4 c3 but better c2 c1" {
		t.Errorf("log = %q", got)
	}
	if body := gitRun(t, root, "log", "-1", "--format=%b", "HEAD~1"); !strings.Contains(body, "with a body") {
		t.Errorf("body = %q", body)
	}
}

func TestRewordRefusesAnEmptyMessage(t *testing.T) {
	root := historyRepo(t, "hist-reword-empty", 3)
	sh, pump, tt := historyShell(t, root)
	sh.openReword(sh.git.commits[0])
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	sh.gitMore.history.msg = "  \n"
	settle(tt, pump, 2)
	before := gitRun(t, root, "rev-parse", "HEAD")
	_ = tt.Click("Reword") // a disabled button must not run
	settle(tt, pump, 2)
	if gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Error("an empty message must not be applied")
	}
}

func TestAmendStagedChangesIntoAnOlderCommit(t *testing.T) {
	root := historyRepo(t, "hist-amend", 4)
	writeFile(t, root+"/late.txt", "belongs with c2\n")
	gitRun(t, root, "add", "late.txt")
	sh, pump, tt := historyShell(t, root)
	if !sh.onlyStagedChanges() {
		t.Fatal("a staged file alone is amendable")
	}
	var c2 gitworkbench.CommitInfo
	for _, c := range sh.git.commits {
		if c.Subject == "c2" {
			c2 = c
		}
	}
	sh.openAmendInto(c2)
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	settle(tt, pump, 2)
	saveEvidence(t, "history-amend", tt)
	if err := tt.Click("Amend"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the amend", func() bool {
		return !sh.git.opBusy && strings.Contains(gitRun(t, root, "show", "--stat", "--format=", hashOf(t, root, "c2")), "late.txt")
	})
	if got := subjects(t, root); got != "c4 c3 c2 c1" {
		t.Errorf("log = %q", got)
	}
}

func TestAmendNeedsOnlyStagedChanges(t *testing.T) {
	root := historyRepo(t, "hist-amend-dirty", 3)
	writeFile(t, root+"/f1.txt", "edited, not staged\n")
	sh, _, _ := historyShell(t, root)
	if sh.onlyStagedChanges() {
		t.Error("an unstaged edit alone is nothing to amend")
	}
	writeFile(t, root+"/staged.txt", "s\n")
	gitRun(t, root, "add", "staged.txt")
	sh.ensureGitSnapshot(true)
	// staged plus an unstaged edit is refused by the kernel, so the entry is off
}

func TestCherryPickSeveralCommitsInTheirOrder(t *testing.T) {
	root := historyRepo(t, "hist-pick", 2)
	gitRun(t, root, "checkout", "-q", "-b", "other")
	for _, n := range []string{"x1", "x2", "x3"} {
		writeFile(t, root+"/"+n+".txt", n+"\n")
		gitRun(t, root, "add", ".")
		gitRun(t, root, "commit", "-q", "-m", n)
	}
	gitRun(t, root, "checkout", "-q", "work")
	sh, pump, tt := historyShell(t, root)
	// chosen in list order, newest first, as commitChoiceHashes returns them
	chosen := []string{hashOf(t, root, "x3"), hashOf(t, root, "x2"), hashOf(t, root, "x1")}
	sh.openCherryPickMany(chosen)
	if !sh.gitMore.history.open {
		t.Fatal("the cherry-pick preview did not open")
	}
	if first := sh.gitMore.history.lines[0]; first.tag != "pick 1" || !strings.Contains(first.text, "x1") {
		t.Errorf("first line = %+v, want x1 picked first", first)
	}
	settle(tt, pump, 2)
	saveEvidence(t, "history-cherry-pick", tt)
	if err := tt.Click("Cherry-pick"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the picks", func() bool { return !sh.git.opBusy && strings.Contains(subjects(t, root), "x3") })
	if got := subjects(t, root); got != "x3 x2 x1 c2 c1" {
		t.Errorf("log = %q, want the three on top in order", got)
	}
}

func TestPreviewRefusalsReachTheUserAsAToast(t *testing.T) {
	root := historyRepo(t, "hist-refuse", 5)
	sh, _, _ := historyShell(t, root)
	sh.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: []string{hashOf(t, root, "c2"), hashOf(t, root, "c4")}})
	if sh.gitMore.history.open || !strings.Contains(sh.pendingToast, "next to each other") {
		t.Errorf("open=%v toast=%q, want a refusal that says why", sh.gitMore.history.open, sh.pendingToast)
	}
}

func TestPublishedCommitsAreWarnedAbout(t *testing.T) {
	root := historyRepo(t, "hist-published", 4)
	remote := t.TempDir() + "/remote.git"
	gitRun(t, root, "init", "-q", "--bare", remote)
	gitRun(t, root, "remote", "add", "origin", remote)
	gitRun(t, root, "push", "-q", "origin", "work:work")
	gitRun(t, root, "fetch", "-q", "origin")
	sh, pump, tt := historyShell(t, root)
	sh.openHistoryRewrite(gitworkbench.HistoryOp{Action: gitworkbench.HistoryDrop, Hashes: []string{hashOf(t, root, "c3")}})
	pump.until(t, "the preview", func() bool { return sh.gitMore.history.open })
	if !strings.Contains(sh.gitMore.history.warn, "already on a remote") {
		t.Errorf("warn = %q", sh.gitMore.history.warn)
	}
	settle(tt, pump, 2)
	saveEvidence(t, "history-published-warning", tt)
}

func TestAConflictingSquashIsCancelledAndTheRepositoryIsUntouched(t *testing.T) {
	root := gitFixture(t, "hist-conflict")
	emptyBranch(t, root, "work")
	for _, step := range [][2]string{{"base", "one\n"}, {"mid", "two\n"}, {"top", "three\n"}} {
		writeFile(t, root+"/shared.txt", step[1])
		gitRun(t, root, "add", ".")
		gitRun(t, root, "commit", "-q", "-m", step[0])
	}
	// squashing "top" into "mid" is fine; moving "top" below "mid" conflicts on the same file
	sh, pump, _ := historyShell(t, root)
	before := gitRun(t, root, "rev-parse", "HEAD")
	op := gitworkbench.HistoryOp{Action: gitworkbench.HistoryMove, Hashes: []string{hashOf(t, root, "top")}, Offset: 1}
	sh.openHistoryRewrite(op)
	pump.until(t, "the preview", func() bool { return sh.gitMore.history.open })
	run := sh.gitMore.history.run
	sh.gitMore.history.open = false
	sh.runGitOp("Move Commits", func(ctx context.Context, root string) (string, error) { return run(ctx, root, "") })
	pump.until(t, "the failed rewrite", func() bool { return !sh.git.opBusy })
	if !strings.Contains(sh.pendingToast, "Move Commits") {
		t.Errorf("toast = %q, want the failure named", sh.pendingToast)
	}
	if got := gitRun(t, root, "rev-parse", "HEAD"); got != before {
		t.Errorf("HEAD moved to %s after a conflicting rewrite, want %s", got, before)
	}
	if st := gitRun(t, root, "status", "--porcelain"); st != "" {
		t.Errorf("working tree not clean afterwards: %q", st)
	}
}

func TestCommitChoiceFollowsCommandAndShiftClicks(t *testing.T) {
	root := historyRepo(t, "hist-choice", 6)
	sh, _, _ := historyShell(t, root)
	entries := sh.git.commits
	h := func(i int) string { return entries[i].Hash }

	if !sh.pickCommit(entries, h(1), 0) {
		t.Fatal("a plain click is a review click")
	}
	sh.pickCommit(entries, h(3), ui.Cmd)
	if got := sh.commitChoiceHashes(entries); len(got) != 2 || got[0] != h(1) || got[1] != h(3) {
		t.Fatalf("⌘ after a plain click chose %v, want rows 1 and 3", got)
	}
	sh.pickCommit(entries, h(3), ui.Cmd)
	if len(sh.commitChoiceHashes(entries)) != 1 {
		t.Error("⌘ on a chosen row takes it out again")
	}
	sh.pickCommit(entries, h(1), 0)
	sh.pickCommit(entries, h(4), ui.Shift)
	if got := sh.commitChoiceHashes(entries); len(got) != 4 || got[0] != h(1) || got[3] != h(4) {
		t.Errorf("⇧ range chose %v, want rows 1 to 4", got)
	}
	if !sh.pickCommit(entries, h(0), 0) || len(sh.commitChoiceHashes(entries)) != 0 {
		t.Error("a plain click clears the choice")
	}
}

func TestCommitRowsInTheChoiceUseTheSelectionWash(t *testing.T) {
	root := historyRepo(t, "hist-wash", 5)
	sh, pump, tt := historyShell(t, root)
	sh.pickCommit(sh.git.commits, sh.git.commits[1].Hash, 0)
	sh.pickCommit(sh.git.commits, sh.git.commits[3].Hash, ui.Shift)
	settle(tt, pump, 3)
	saveEvidence(t, "history-choice", tt)
}

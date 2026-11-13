package nativeui

/**
 * [INPUT]: git_patch_ui.go、真实临时仓库
 * [OUTPUT]: Custom Patch 界面的合同：整文件与按行两种选择的规则（点选 / ⌘ 追加 / ⇧ 连选 / 整文件被行选收窄 / 换提交即清空）、哪些提交不可用、Remove / Split / Move / Undo 四个动作先预览再执行且结果与选择一致、冲突或对不上时仓库原样；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (12) 条的界面防线；内核合同在 solco-gitkit gitworkbench/custom_patch*_test.go 与 engine/patch_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// patchRepo: c1 adds base.txt; c2 rewrites l2 and adds l4 there and adds new.txt;
// c3 adds f3.txt on top.
func patchRepo(t *testing.T, name string) string {
	t.Helper()
	root := gitFixture(t, name)
	emptyBranch(t, root, "work")
	commit := func(msg string, files map[string]string) {
		for p, body := range files {
			writeFile(t, root+"/"+p, body)
		}
		gitRun(t, root, "add", ".")
		gitRun(t, root, "commit", "-q", "-m", msg)
	}
	commit("c1", map[string]string{"base.txt": "l1\nl2\nl3\n"})
	commit("c2", map[string]string{"base.txt": "l1\nl2x\nl3\nl4\n", "new.txt": "new\n"})
	commit("c3", map[string]string{"f3.txt": "three\n"})
	return root
}

// reviewing opens the commit's review and waits for its files to be laid out.
func reviewing(t *testing.T, sh *Shell, pump *uiPump, tt *ui.Tester, hash string) {
	t.Helper()
	sh.reviewCommit(hash)
	pump.until(t, "the commit review", func() bool {
		settle(tt, pump, 1)
		return sh.gdReviewingCommit() && sh.git.commitSnap != nil && len(sh.git.gdFiles) > 0
	})
}

func gdFileOf(t *testing.T, sh *Shell, path string) *gdFile {
	t.Helper()
	for _, f := range sh.git.gdFiles {
		if f.cf.Path == path {
			return f
		}
	}
	t.Fatalf("%s is not among the shown files", path)
	return nil
}

// lineOf finds the index of the first changed line with this text in a hunk.
func lineOf(t *testing.T, f *gdFile, hunk int, text string) int {
	t.Helper()
	for i, l := range f.hunks()[hunk].Lines {
		if l.Kind != gitworkbench.KindContext && strings.TrimSpace(l.Text) == text {
			return i
		}
	}
	t.Fatalf("no changed line %q in %s", text, f.cf.Path)
	return -1
}

func patchShell(t *testing.T, name string) (*Shell, *uiPump, *ui.Tester, string) {
	root := patchRepo(t, name)
	sh, pump, tt := historyShell(t, root)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	return sh, pump, tt, root
}

func TestPatchSelectionRules(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-rules")
	base := gdFileOf(t, sh, "base.txt")
	l2x, l4 := lineOf(t, base, 0, "l2x"), lineOf(t, base, 0, "l4")

	sh.patchPickLine(base, 0, l4, 0)
	if !sh.patchPicked(base, 0, l4) || sh.patchPicked(base, 0, l2x) {
		t.Fatal("a plain click picks just that line")
	}
	sh.patchPickLine(base, 0, l4, 0)
	if files, _ := sh.patchCounts(); files != 0 {
		t.Error("clicking the only picked line again clears the choice")
	}
	sh.patchPickLine(base, 0, l4, 0)
	sh.patchPickLine(base, 0, l2x, ui.Cmd)
	if _, lines := sh.patchCounts(); lines != 2 {
		t.Errorf("⌘ adds a line: %d picked, want 2", lines)
	}
	sh.patchPickLine(base, 0, l2x, ui.Cmd)
	if sh.patchPicked(base, 0, l2x) {
		t.Error("⌘ on a picked line takes it out")
	}
	sh.clearPatchSelection()
	sh.patchPickLine(base, 0, l2x, 0)
	sh.patchPickLine(base, 0, l4, ui.Shift)
	if _, lines := sh.patchCounts(); lines != 2 || !sh.patchPicked(base, 0, l2x) || !sh.patchPicked(base, 0, l4) {
		t.Errorf("⇧ picks the changed lines from l2x to l4 (the context line between is no pick): got %d", lines)
	}

	// a whole-file choice picks every line; a line click narrows it to that line
	sh.clearPatchSelection()
	sh.togglePatchFile(base.cf)
	if !sh.patchPicked(base, 0, l2x) || !sh.patchPicked(base, 0, l4) {
		t.Fatal("a whole file has every changed line picked")
	}
	if got := sh.patchBadge("base.txt"); got != "In patch" {
		t.Errorf("badge = %q", got)
	}
	sh.patchPickLine(base, 0, l4, 0)
	if sh.patchPicked(base, 0, l2x) || !sh.patchPicked(base, 0, l4) {
		t.Error("a line click on a whole-file choice narrows it to that line")
	}
	if got := sh.patchBadge("base.txt"); got != "1 line in patch" {
		t.Errorf("badge = %q", got)
	}

	// a hunk button picks and clears the hunk
	sh.clearPatchSelection()
	sh.patchSetHunk(base, 0, true)
	if !sh.patchPicked(base, 0, l2x) || !sh.patchPicked(base, 0, l4) {
		t.Error("Add Hunk to Patch picks the hunk's changes")
	}
	sh.patchSetHunk(base, 0, false)
	if files, _ := sh.patchCounts(); files != 0 {
		t.Error("clearing the hunk leaves nothing chosen")
	}

	// the choice belongs to the commit under review
	sh.togglePatchFile(base.cf)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c3"))
	if files, _ := sh.patchCounts(); files != 0 {
		t.Error("another commit starts with an empty choice")
	}
}

func TestPatchIsOfferedOnlyForOrdinaryCommitsOfTheBranch(t *testing.T) {
	root := patchRepo(t, "patch-eligible")
	gitRun(t, root, "checkout", "-q", "-b", "side", "HEAD~1")
	writeFile(t, root+"/s.txt", "s\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "on side")
	gitRun(t, root, "checkout", "-q", "work")
	gitRun(t, root, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	gitRun(t, root, "checkout", "-q", "-b", "other", "work~1")
	writeFile(t, root+"/o.txt", "o\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "on other")
	gitRun(t, root, "checkout", "-q", "work")
	sh, pump, tt := historyShell(t, root)

	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	if ok, _ := sh.patchEligible(); !ok {
		t.Error("an ordinary commit of the branch is eligible")
	}
	reviewing(t, sh, pump, tt, hashOf(t, root, "merge side"))
	if ok, why := sh.patchEligible(); ok || !strings.Contains(why, "merge") {
		t.Errorf("a merge commit must be refused with its reason: ok=%v why=%q", ok, why)
	}
	reviewing(t, sh, pump, tt, hashOf(t, root, "on other"))
	if ok, why := sh.patchEligible(); ok || !strings.Contains(why, "checked-out") {
		t.Errorf("a commit of another branch must be refused: ok=%v why=%q", ok, why)
	}
}

// confirmPatch opens the sheet of an action, checks what it says and confirms.
func confirmPatch(t *testing.T, sh *Shell, pump *uiPump, tt *ui.Tester, root string, action gitworkbench.PatchAction, dest, button string, evidence string) {
	t.Helper()
	before := gitRun(t, root, "rev-parse", "HEAD")
	sh.openPatchSheet(action, dest)
	pump.until(t, "the confirm sheet", func() bool { return sh.gitMore.history.open })
	settle(tt, pump, 2)
	if evidence != "" {
		saveEvidence(t, evidence, tt)
	}
	if gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Fatal("the preview changed the repository")
	}
	if err := tt.Click(button); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the rewrite", func() bool { return !sh.git.opBusy && gitRun(t, root, "rev-parse", "HEAD") != before })
}

func TestRemovingPickedLinesTakesOutJustThose(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-remove-lines")
	base := gdFileOf(t, sh, "base.txt")
	sh.patchPickLine(base, 0, lineOf(t, base, 0, "l4"), 0)
	settle(tt, pump, 2)
	saveEvidence(t, "patch-bar", tt)

	confirmPatch(t, sh, pump, tt, root, gitworkbench.PatchRemove, "", "Remove", "patch-remove-preview")
	if got := subjects(t, root); got != "c3 c2 c1" {
		t.Fatalf("log = %q", got)
	}
	if got := gitRun(t, root, "show", hashOf(t, root, "c2")+":base.txt"); got != "l1\nl2x\nl3" {
		t.Errorf("base.txt in c2 = %q, want only l4 gone", got)
	}
	if got := gitRun(t, root, "show", hashOf(t, root, "c2")+":new.txt"); got != "new" {
		t.Errorf("new.txt = %q, the other file must stay", got)
	}
	if files, _ := sh.patchCounts(); files != 0 {
		t.Error("the choice is spent after the rewrite")
	}
}

func TestRemovingAWholeFileDropsIt(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-remove-file")
	sh.togglePatchFile(gdFileOf(t, sh, "new.txt").cf)
	confirmPatch(t, sh, pump, tt, root, gitworkbench.PatchRemove, "", "Remove", "")
	if got := gitRun(t, root, "ls-tree", "-r", "--name-only", hashOf(t, root, "c2")); strings.Contains(got, "new.txt") || !strings.Contains(got, "base.txt") {
		t.Errorf("c2 holds %q", got)
	}
}

func TestSplittingNeedsAMessageAndMakesANewCommit(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-split")
	sh.togglePatchFile(gdFileOf(t, sh, "new.txt").cf)
	sh.openPatchSheet(gitworkbench.PatchSplit, "")
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	if !sh.gitMore.history.needsMsg {
		t.Fatal("a split asks for the new commit's message")
	}
	before := gitRun(t, root, "rev-parse", "HEAD")
	settle(tt, pump, 2)
	_ = tt.Click("Create Commit") // no message: a disabled button must not run
	settle(tt, pump, 2)
	if gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Fatal("a split without a message must not run")
	}
	sh.gitMore.history.msg = "add new.txt"
	settle(tt, pump, 2)
	saveEvidence(t, "patch-split", tt)
	if err := tt.Click("Create Commit"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the split", func() bool { return !sh.git.opBusy && gitRun(t, root, "rev-parse", "HEAD") != before })
	if got := subjects(t, root); got != "c3 add new.txt c2 c1" {
		t.Fatalf("log = %q", got)
	}
	if got := gitRun(t, root, "show", "--stat", "--format=", hashOf(t, root, "add new.txt")); !strings.Contains(got, "new.txt") || strings.Contains(got, "base.txt") {
		t.Errorf("the new commit holds %q", got)
	}
}

func TestMovingToAnOlderCommit(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-move")
	sh.togglePatchFile(gdFileOf(t, sh, "new.txt").cf)
	sh.openPatchDest()
	if !sh.gitMore.patchDest.open || len(sh.gitMore.patchDest.commits) != 2 {
		t.Fatalf("the picker offers the branch's other commits: %+v", sh.gitMore.patchDest)
	}
	settle(tt, pump, 2)
	saveEvidence(t, "patch-move-dest", tt)
	sh.gitMore.patchDest.open = false
	confirmPatch(t, sh, pump, tt, root, gitworkbench.PatchMove, hashOf(t, root, "c1"), "Move", "")
	if got := gitRun(t, root, "ls-tree", "-r", "--name-only", hashOf(t, root, "c1")); !strings.Contains(got, "new.txt") {
		t.Errorf("c1 holds %q, want new.txt moved in", got)
	}
	if got := gitRun(t, root, "show", "--stat", "--format=", hashOf(t, root, "c2")); strings.Contains(got, "new.txt") {
		t.Errorf("c2 still changes new.txt: %q", got)
	}
}

func TestUndoingPutsTheChangesBackStaged(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-undo")
	sh.togglePatchFile(gdFileOf(t, sh, "new.txt").cf)
	confirmPatch(t, sh, pump, tt, root, gitworkbench.PatchUndo, "", "Undo", "")
	if got := gitRun(t, root, "status", "--porcelain"); !strings.Contains(got, "A  new.txt") {
		t.Errorf("status = %q, want new.txt staged", got)
	}
	if got := subjects(t, root); got != "c3 c2 c1" {
		t.Errorf("log = %q", got)
	}
}

func TestAPatchThatConflictsLeavesTheRepositoryAsItWas(t *testing.T) {
	root := patchRepo(t, "patch-conflict")
	// c4 builds on the lines c2 added, so taking them out of c2 cannot replay c4
	writeFile(t, root+"/base.txt", "l1\nl2x\nl3\nl4\nl5\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c4")
	sh, pump, tt := historyShell(t, root)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	base := gdFileOf(t, sh, "base.txt")
	sh.patchPickLine(base, 0, lineOf(t, base, 0, "l4"), 0)
	before := gitRun(t, root, "rev-parse", "HEAD")

	sh.openPatchSheet(gitworkbench.PatchRemove, "")
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	settle(tt, pump, 2)
	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the attempt", func() bool { return !sh.git.opBusy && sh.pendingToast != "" })
	if gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Error("a conflicting patch must leave the branch where it was")
	}
	if got := gitRun(t, root, "status", "--porcelain"); got != "" {
		t.Errorf("the working tree is not clean: %q", got)
	}
}

func TestAPickThatNoLongerMatchesIsRefused(t *testing.T) {
	sh, pump, tt, root := patchShell(t, "patch-mismatch")
	base := gdFileOf(t, sh, "base.txt")
	i := lineOf(t, base, 0, "l4")
	sh.patchPickLine(base, 0, i, 0)
	// the shown diff and Git's reading of it disagree about this line
	sh.git.gdFiles[indexOfFile(sh, "base.txt")].cf.Hunks[0].Lines[i].Text = "something else"
	sh.openPatchSheet(gitworkbench.PatchRemove, "")
	pump.until(t, "the sheet", func() bool { return sh.gitMore.history.open })
	settle(tt, pump, 2)
	before := gitRun(t, root, "rev-parse", "HEAD")
	if err := tt.Click("Remove"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the refusal", func() bool { return !sh.git.opBusy && sh.pendingToast != "" })
	if gitRun(t, root, "rev-parse", "HEAD") != before {
		t.Error("a mismatching pick must change nothing")
	}
}

func indexOfFile(sh *Shell, path string) int {
	for i, f := range sh.git.gdFiles {
		if f.cf.Path == path {
			return i
		}
	}
	return -1
}

func TestPickedLinesOfARenamedFileAreRemoved(t *testing.T) {
	root := gitFixture(t, "patch-rename")
	emptyBranch(t, root, "work")
	writeFile(t, root+"/a.txt", "one\ntwo\nthree\nfour\nfive\nsix\nseven\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c1")
	gitRun(t, root, "mv", "a.txt", "b.txt")
	writeFile(t, root+"/b.txt", "one\nTWO\nthree\nfour\nfive\nsix\nseven\neight\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c2")
	writeFile(t, root+"/c.txt", "c\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "c3")
	sh, pump, tt := historyShell(t, root)
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	b := gdFileOf(t, sh, "b.txt")
	if b.cf.OldPath != "a.txt" {
		t.Fatalf("the fixture is no rename: %+v", b.cf)
	}
	sh.patchPickLine(b, 0, lineOf(t, b, 0, "eight"), 0)
	confirmPatch(t, sh, pump, tt, root, gitworkbench.PatchRemove, "", "Remove", "")
	if got := gitRun(t, root, "show", hashOf(t, root, "c2")+":b.txt"); got != "one\nTWO\nthree\nfour\nfive\nsix\nseven" {
		t.Errorf("b.txt in c2 = %q, want only the added line gone", got)
	}
}

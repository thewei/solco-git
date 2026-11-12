package nativeui

/**
 * [INPUT]: git_conflict.go、真实临时仓库里的真实合并冲突、headless 渲染
 * [OUTPUT]: 冲突编辑界面的合同：右键 Resolve Conflicts… 打开并读出两个冲突块、点 Current / Incoming 改写 Result 且点 Apply & Stage 前不落盘、手改 Result 后块按钮停用而 Reset Result 恢复、Apply 暂存结果并退出冲突、残留标记时 Apply 停用、一方删除的文件只给原因与整文件选择、Recovery 里能重开
 * [POS]: 2026-11-12 决定第 (3) 条冲突三栏编辑的界面防线；底层合同在 solco-gitkit gitworkbench/conflict_test.go；SOLCOGIT_EVIDENCE=<dir> 保留 PNG 对设计
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

const conflictBodyText = "line a\nline b\nline c\nline d\nline e\nline f\nline g\nline h\nline i\nline j\n"

// conflictFixture pauses a merge in a repository named name: f.txt has two
// conflicting hunks (a and i) and gone.txt was deleted on the incoming side.
func conflictFixture(t *testing.T, name string) string {
	t.Helper()
	root := gitFixture(t, name)
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "Fixture state")
	writeFile(t, root+"/f.txt", conflictBodyText)
	writeFile(t, root+"/gone.txt", "gone\n")
	gitRun(t, root, "add", "f.txt", "gone.txt")
	gitRun(t, root, "commit", "-q", "-m", "add f")
	gitRun(t, root, "switch", "-q", "-c", "topic")
	writeFile(t, root+"/f.txt", strings.NewReplacer("line a", "topic a", "line i", "topic i").Replace(conflictBodyText))
	gitRun(t, root, "rm", "-q", "gone.txt")
	gitRun(t, root, "add", "f.txt")
	gitRun(t, root, "commit", "-q", "-m", "topic")
	gitRun(t, root, "switch", "-q", "main")
	writeFile(t, root+"/f.txt", strings.NewReplacer("line a", "main a", "line i", "main i").Replace(conflictBodyText))
	writeFile(t, root+"/gone.txt", "changed on main\n")
	gitRun(t, root, "add", "f.txt", "gone.txt")
	gitRun(t, root, "commit", "-q", "-m", "main")
	cmd := exec.Command("git", "merge", "topic")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("expected a conflict: %s", out)
	}
	return root
}

func TestConflictEditorDecidesHunksAndStagesTheResult(t *testing.T) {
	root := conflictFixture(t, "conflict-edit")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1400, 900, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)

	sh.openConflictEditor("f.txt")
	pump.until(t, "the conflict", func() bool { return !sh.gitMore.conflict.loading })
	cs := &sh.gitMore.conflict
	if cs.err != "" || !cs.sides.Editable || len(cs.hunks) != 2 {
		t.Fatalf("state = err %q editable %v hunks %d", cs.err, cs.sides.Editable, len(cs.hunks))
	}
	settle(tt, pump, 3)
	saveEvidence(t, "conflict-open", tt)
	if !strings.Contains(cs.result.Buffer.String(), "<<<<<<< Current") || cs.undecided() != 2 {
		t.Fatal("a fresh editor should show both conflicts as undecided in the Result")
	}

	if err := tt.Click("Incoming"); err != nil { // the first hunk's button
		t.Fatal(err)
	}
	settle(tt, pump, 2)
	if cs.choices[0] != 1+1 { // ChoiceTheirs
		t.Fatalf("first hunk choice = %v after clicking Incoming", cs.choices[0])
	}
	if got := cs.result.Buffer.String(); !strings.Contains(got, "topic a\n") || strings.Count(got, "<<<<<<<") != 1 {
		t.Fatalf("Result after the first choice:\n%s", got)
	}
	if got := readFileT(t, root+"/f.txt"); !strings.Contains(got, "<<<<<<<") {
		t.Fatal("choosing a hunk must not touch the working tree before Apply")
	}

	cs.choose(2, 1) // ChoiceOurs
	settle(tt, pump, 2)
	saveEvidence(t, "conflict-decided", tt)
	if want := strings.NewReplacer("line a", "topic a", "line i", "main i").Replace(conflictBodyText); cs.result.Buffer.String() != want {
		t.Fatalf("Result:\n%s\nwant:\n%s", cs.result.Buffer.String(), want)
	}

	// a hand edit locks the hunk buttons until Reset Result
	cs.result.Buffer.Set(cs.derived + "typed by hand\n")
	if !cs.edited() {
		t.Fatal("the edit was not noticed")
	}
	cs.choose(1, 1)
	if cs.choices[0] == 1 {
		t.Error("a hunk button changed a hand-edited Result")
	}
	settle(tt, pump, 2)
	saveEvidence(t, "conflict-edited", tt)
	cs.resetResult()
	if cs.edited() {
		t.Error("Reset Result left the Result edited")
	}

	if err := tt.Click("Apply & Stage"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the apply", func() bool { return !cs.open && !sh.git.opBusy })
	want := strings.NewReplacer("line a", "topic a", "line i", "main i").Replace(conflictBodyText)
	if got := readFileT(t, root+"/f.txt"); got != want {
		t.Errorf("f.txt = %q", got)
	}
	if out := gitRun(t, root, "diff", "--name-only", "--diff-filter=U"); strings.Contains(out, "f.txt") {
		t.Errorf("f.txt is still unmerged: %q", out)
	}
}

func TestConflictEditorHoldsApplyWhileMarkersRemain(t *testing.T) {
	root := conflictFixture(t, "conflict-markers")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1400, 900, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)
	sh.openConflictEditor("f.txt")
	pump.until(t, "the conflict", func() bool { return !sh.gitMore.conflict.loading })
	settle(tt, pump, 3)
	if err := tt.Click("Apply & Stage"); err == nil && !sh.gitMore.conflict.open {
		t.Fatal("Apply & Stage closed the editor with conflicts undecided")
	}
	if out := gitRun(t, root, "diff", "--name-only", "--diff-filter=U"); !strings.Contains(out, "f.txt") {
		t.Fatal("an undecided file was staged")
	}
}

func TestConflictEditorOffersOnlyWholeFilesWhenASideDeletedIt(t *testing.T) {
	root := conflictFixture(t, "conflict-whole")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1400, 900, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)
	sh.openConflictEditor("gone.txt")
	pump.until(t, "the conflict", func() bool { return !sh.gitMore.conflict.loading })
	settle(tt, pump, 3)
	cs := &sh.gitMore.conflict
	if cs.sides.Editable || !strings.Contains(cs.sides.Reason, "deleted") {
		t.Fatalf("sides = %+v", cs.sides)
	}
	saveEvidence(t, "conflict-whole-file", tt)
	if err := tt.Click("Take Incoming Version"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the resolution", func() bool { return !cs.open && !sh.git.opBusy })
	if _, err := os.Stat(root + "/gone.txt"); !os.IsNotExist(err) {
		t.Errorf("taking the incoming (deleted) side must delete the file (stat err = %v)", err)
	}
}

func TestResolvedConflictCanBeReopenedFromRecovery(t *testing.T) {
	root := conflictFixture(t, "conflict-recover")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1400, 900, true)
	sh := openAndLoad(t, w, pump, root)
	settle(tt, pump, 3)
	sh.openConflictEditor("f.txt")
	pump.until(t, "the conflict", func() bool { return !sh.gitMore.conflict.loading })
	cs := &sh.gitMore.conflict
	cs.choose(1, 1)
	cs.choose(2, 1)
	sh.applyConflictResult(cs.result.Buffer.String())
	pump.until(t, "the apply", func() bool { return !sh.git.opBusy })

	sh.openRecoverySheet()
	pump.until(t, "the list", func() bool { return !sh.gitMore.recovery.loading })
	ops := sh.gitMore.recovery.ops
	if len(ops) != 1 || ops[0].Kind != "resolve-conflict" {
		t.Fatalf("recovery list = %+v", ops)
	}
	sh.recoverOperation(ops[0])
	pump.until(t, "the recovery", func() bool { return !sh.git.opBusy })
	if out := gitRun(t, root, "diff", "--name-only", "--diff-filter=U"); !strings.Contains(out, "f.txt") {
		t.Errorf("the conflict was not reopened: %q", out)
	}
	if !strings.Contains(readFileT(t, root+"/f.txt"), "<<<<<<<") {
		t.Error("the marker file was not put back")
	}
}

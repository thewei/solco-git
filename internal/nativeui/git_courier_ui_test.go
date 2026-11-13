package nativeui

/**
 * [INPUT]: git_courier_ui.go、真实临时仓库与它的第二个 worktree
 * [OUTPUT]: Change Courier 界面的合同：从文件菜单 / 已选行 / Custom Patch 条送到另一个 worktree；先预检再送，不干净时按钮不可用并说明原因；送达只进目标工作区且源不变；Move 后源里这些改动消失（整文件、行、提交里的文件）；没有其它 worktree 时给出原因而不开面板；换目标即重新预检；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: 2026-11-12 决定第 (13) 条的界面防线；内核合同在 solco-gitkit gitworkbench/courier_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

func courierDoc(edit map[int]string) string {
	var b strings.Builder
	for i := 1; i <= 30; i++ {
		if t, ok := edit[i]; ok {
			b.WriteString(t + "\n")
			continue
		}
		b.WriteString("line " + itoa(i) + "\n")
	}
	return b.String()
}

// courierRepo: doc.txt committed on an orphan branch, plus a second worktree.
func courierRepo(t *testing.T, name string, dests ...string) (root string, dest map[string]string) {
	t.Helper()
	root = gitFixture(t, name)
	emptyBranch(t, root, "work")
	writeFile(t, root+"/doc.txt", courierDoc(nil))
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "doc")
	dest = map[string]string{}
	for _, d := range dests {
		p := filepath.Join(t.TempDir(), d)
		gitRun(t, root, "worktree", "add", "-q", "-b", d, p)
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		dest[d] = real
	}
	return root, dest
}

// courierShell opens root on Changes with its files laid out and its
// worktrees read.
func courierShell(t *testing.T, root string, wantWorktrees int) (*Shell, *uiPump, *ui.Tester) {
	t.Helper()
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the worktrees and the changed files", func() bool {
		settle(tt, pump, 1)
		return len(sh.git.worktrees) >= wantWorktrees && len(sh.git.gdFiles) > 0
	})
	return sh, pump, tt
}

func cfOf(t *testing.T, sh *Shell, path string) *gdFile { return gdFileOf(t, sh, path) }

func waitPreview(t *testing.T, sh *Shell, pump *uiPump, tt *ui.Tester) {
	t.Helper()
	pump.until(t, "the preview", func() bool {
		settle(tt, pump, 1)
		c := &sh.gitMore.courier
		return c.open && !c.building && (c.preview != nil || c.problem != "" || c.err != "")
	})
	settle(tt, pump, 2)
}

func TestSendingAFileLandsItUnstagedInTheOtherWorktree(t *testing.T) {
	root, dest := courierRepo(t, "courier-file", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	sh, pump, tt := courierShell(t, root, 2)

	sh.openCourierFile(cfOf(t, sh, "doc.txt").cf)
	waitPreview(t, sh, pump, tt)
	c := sh.gitMore.courier
	if c.preview == nil || !c.preview.Clean || len(c.dests) != 1 || c.dests[0].Path != dest["feature2"] {
		t.Fatalf("sheet = %+v", c)
	}
	saveEvidence(t, "courier-file", tt)
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the delivery", func() bool {
		return !sh.git.opBusy && gitRun(t, dest["feature2"], "status", "--porcelain") != ""
	})
	if got := readFile(t, dest["feature2"]+"/doc.txt"); got != courierDoc(map[int]string{3: "three"}) {
		t.Errorf("destination doc.txt = %q", got)
	}
	if got := gitRun(t, dest["feature2"], "status", "--porcelain"); got != "M doc.txt" {
		t.Errorf("destination status = %q, want an unstaged modification", got)
	}
	if got := gitRun(t, root, "status", "--porcelain"); got != "M doc.txt" {
		t.Errorf("the source changed: %q", got)
	}
	if !strings.Contains(sh.pendingToast, "Sent to feature2") {
		t.Errorf("toast = %q", sh.pendingToast)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMovingAFileTakesItOutOfTheSource(t *testing.T) {
	root, dest := courierRepo(t, "courier-move", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	writeFile(t, root+"/fresh.txt", "brand new\n")
	sh, pump, tt := courierShell(t, root, 2)

	for _, path := range []string{"doc.txt", "fresh.txt"} {
		sh.openCourierFile(cfOf(t, sh, path).cf)
		waitPreview(t, sh, pump, tt)
		if !sh.gitMore.courier.moveOK {
			t.Fatalf("%s: Move should be offered for an unstaged file", path)
		}
		sh.gitMore.courier.move = true
		settle(tt, pump, 2)
		if path == "doc.txt" {
			saveEvidence(t, "courier-move", tt)
		}
		if err := tt.Click("Move"); err != nil {
			t.Fatal(err)
		}
		pump.until(t, path+" to move", func() bool { return !sh.git.opBusy && strings.Contains(sh.pendingToast, "Moved") })
		sh.pendingToast = ""
	}
	if got := gitRun(t, root, "status", "--porcelain"); got != "" {
		t.Errorf("the source still has changes: %q", got)
	}
	if got := readFile(t, dest["feature2"]+"/fresh.txt"); got != "brand new\n" {
		t.Errorf("fresh.txt = %q", got)
	}
	if got := readFile(t, dest["feature2"]+"/doc.txt"); !strings.Contains(got, "three") {
		t.Errorf("doc.txt = %q", got)
	}
}

func TestAStagedFileCanBeSentButNotMoved(t *testing.T) {
	root, _ := courierRepo(t, "courier-staged", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	gitRun(t, root, "add", "doc.txt")
	sh, pump, tt := courierShell(t, root, 2)
	sh.openCourierFile(cfOf(t, sh, "doc.txt").cf)
	waitPreview(t, sh, pump, tt)
	if sh.gitMore.courier.moveOK || !strings.Contains(sh.gitMore.courier.moveWhy, "staged") {
		t.Errorf("moveOK=%v why=%q, want Move off for a staged file", sh.gitMore.courier.moveOK, sh.gitMore.courier.moveWhy)
	}
	if !sh.gitMore.courier.preview.Clean {
		t.Error("a staged file still travels")
	}
	// the switch cannot be forced on from outside: a staged file is never taken out
	sh.gitMore.courier.move = true
	settle(tt, pump, 2)
	if err := tt.Click("Send"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the delivery", func() bool { return !sh.git.opBusy && strings.Contains(sh.pendingToast, "Sent") })
	if got := gitRun(t, root, "status", "--porcelain"); got != "M  doc.txt" {
		t.Errorf("the source = %q, want its staged change kept", got)
	}
}

func TestStagedLinesAreSentButNotMoved(t *testing.T) {
	root, _ := courierRepo(t, "courier-staged-lines", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	gitRun(t, root, "add", "doc.txt")
	sh, pump, tt := courierShell(t, root, 2)
	f := cfOf(t, sh, "doc.txt")
	picked := map[int]bool{}
	for i, l := range f.hunks()[0].Lines {
		if l.Kind != gitworkbench.KindContext {
			picked[i] = true
		}
	}
	sh.openCourierLines(f, 0, picked)
	waitPreview(t, sh, pump, tt)
	if c := sh.gitMore.courier; c.moveOK || !strings.Contains(c.moveWhy, "staged") || c.preview == nil || !c.preview.Clean {
		t.Errorf("sheet = moveOK %v why %q preview %+v", c.moveOK, c.moveWhy, c.preview)
	}
}

func TestPickedLinesAreSentAndMovedOnTheirOwn(t *testing.T) {
	root, dest := courierRepo(t, "courier-lines", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three", 25: "twenty-five"}))
	sh, pump, tt := courierShell(t, root, 2)
	f := cfOf(t, sh, "doc.txt")
	if len(f.hunks()) != 2 {
		t.Fatalf("hunks = %d, want 2 far apart", len(f.hunks()))
	}
	picked := map[int]bool{}
	for i, l := range f.hunks()[1].Lines {
		if l.Kind != gitworkbench.KindContext {
			picked[i] = true
		}
	}
	if len(picked) != 2 {
		t.Fatalf("picked %v, want the removed and the added line", picked)
	}
	sh.openCourierLines(f, 1, picked)
	waitPreview(t, sh, pump, tt)
	if !sh.gitMore.courier.moveOK {
		t.Fatalf("Move should be offered for unstaged lines: %q", sh.gitMore.courier.moveWhy)
	}
	sh.gitMore.courier.move = true
	settle(tt, pump, 2)
	saveEvidence(t, "courier-lines", tt)
	if err := tt.Click("Move"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the move", func() bool { return !sh.git.opBusy && strings.Contains(sh.pendingToast, "Moved") })
	if got := readFile(t, dest["feature2"]+"/doc.txt"); got != courierDoc(map[int]string{25: "twenty-five"}) {
		t.Errorf("destination = %q, want only the second change", got)
	}
	if got := readFile(t, root+"/doc.txt"); got != courierDoc(map[int]string{3: "three"}) {
		t.Errorf("source = %q, want only the first change left", got)
	}
}

func TestWhatDoesNotFitCannotBeSent(t *testing.T) {
	root, dest := courierRepo(t, "courier-nofit", "feature2")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	other := courierDoc(map[int]string{3: "something else"})
	writeFile(t, dest["feature2"]+"/doc.txt", other)
	sh, pump, tt := courierShell(t, root, 2)
	sh.openCourierFile(cfOf(t, sh, "doc.txt").cf)
	waitPreview(t, sh, pump, tt)
	if p := sh.gitMore.courier.preview; p == nil || p.Clean || !strings.Contains(p.Problem, "doc.txt") {
		t.Fatalf("preview = %+v, want a refusal naming doc.txt", p)
	}
	saveEvidence(t, "courier-nofit", tt)
	_ = tt.Click("Send") // disabled: must not run
	settle(tt, pump, 2)
	if got := readFile(t, dest["feature2"]+"/doc.txt"); got != other {
		t.Error("a refused send changed the destination")
	}
}

func TestChangingTheDestinationChecksAgainstThatOne(t *testing.T) {
	root, dest := courierRepo(t, "courier-two", "alpha", "beta")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	writeFile(t, dest["beta"]+"/doc.txt", courierDoc(map[int]string{3: "different"}))
	sh, pump, tt := courierShell(t, root, 3)
	sh.openCourierFile(cfOf(t, sh, "doc.txt").cf)
	waitPreview(t, sh, pump, tt)
	c := &sh.gitMore.courier
	if len(c.dests) != 2 {
		t.Fatalf("destinations = %d, want the two other worktrees", len(c.dests))
	}
	first := c.pick
	other := 1 - first
	sh.courierPickDest(other)
	pump.until(t, "the second preview", func() bool { settle(tt, pump, 1); return !c.checking && c.preview != nil })
	fits := func(i int) bool { return c.dests[i].Path != dest["beta"] }
	if c.pick != other || c.preview.Clean != fits(other) {
		t.Errorf("after choosing %s the preview is clean=%v", courierLabel(c.dests[other]), c.preview.Clean)
	}
}

func TestSendingNeedsAnotherWorktree(t *testing.T) {
	root, _ := courierRepo(t, "courier-alone")
	writeFile(t, root+"/doc.txt", courierDoc(map[int]string{3: "three"}))
	sh, _, _ := courierShell(t, root, 1)
	sh.openCourierFile(cfOf(t, sh, "doc.txt").cf)
	if sh.gitMore.courier.open || !strings.Contains(sh.pendingToast, "no other worktree") {
		t.Errorf("open=%v toast=%q, want a reason and no sheet", sh.gitMore.courier.open, sh.pendingToast)
	}
}

func TestCustomPatchChoiceOfACommitCanBeSentAndMoved(t *testing.T) {
	root := patchRepo(t, "courier-commit")
	dest := filepath.Join(t.TempDir(), "elsewhere")
	gitRun(t, root, "worktree", "add", "-q", "-b", "elsewhere", dest, "HEAD~2") // at c1: where c2's changes fit
	dest, _ = filepath.EvalSymlinks(dest)
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	sh.showGitCommits()
	pump.until(t, "the commits and worktrees", func() bool { return len(sh.git.commits) > 0 && len(sh.git.worktrees) > 1 })
	reviewing(t, sh, pump, tt, hashOf(t, root, "c2"))
	sh.togglePatchFile(gdFileOf(t, sh, "new.txt").cf)
	base := gdFileOf(t, sh, "base.txt")
	sh.patchPickLine(base, 0, lineOf(t, base, 0, "l4"), ui.Cmd)

	sh.openCourierPatch()
	waitPreview(t, sh, pump, tt)
	c := sh.gitMore.courier
	if c.preview == nil || !c.preview.Clean || len(c.preview.Files) != 2 || !c.moveOK {
		t.Fatalf("sheet = %+v", c)
	}
	sh.gitMore.courier.move = true
	settle(tt, pump, 2)
	saveEvidence(t, "courier-commit", tt)
	if err := tt.Click("Move"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the move", func() bool { return !sh.git.opBusy && strings.Contains(sh.pendingToast, "Moved") })
	if got := readFile(t, dest+"/new.txt"); got != "new\n" {
		t.Errorf("new.txt in the destination = %q", got)
	}
	if got := readFile(t, dest+"/base.txt"); got != "l1\nl2\nl3\nl4\n" {
		t.Errorf("base.txt in the destination = %q", got)
	}
	if got := gitRun(t, root, "show", "--stat", "--format=", hashOf(t, root, "c2")); strings.Contains(got, "new.txt") {
		t.Errorf("c2 still changes new.txt: %q", got)
	}
	if got := subjects(t, root); got != "c3 c2 c1" {
		t.Errorf("log = %q", got)
	}
}

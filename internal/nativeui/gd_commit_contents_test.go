package nativeui

/**
 * [INPUT]: gd_state.go（loadGdContents/gdReadFile）、真实临时仓库（提交/贮藏之后工作区又改了同一文件）、reviewAndWait
 * [OUTPUT]: 提交审阅的内容合同：未改动行读该提交两侧的 blob（提交、贮藏、改名），Compare to Local Changes 读提交对工作区，而不是 HEAD/工作区——否则文件尾部会冒出提交里没有的"幽灵上下文行"
 * [POS]: 2026-11-08 实机发现的 Commits diff 尾部幽灵行的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func numberedLines(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

// reviewedFile waits for the open review's file of that path and its
// contents.
func reviewedFile(t *testing.T, pump *uiPump, sh *Shell, path string) *gdFile {
	t.Helper()
	find := func() *gdFile {
		for _, g := range sh.git.gdFiles {
			if g.cf.Path == path {
				return g
			}
		}
		return nil
	}
	pump.until(t, "the review's files", func() bool { return find() != nil })
	sh.loadGdContents()
	// The file object may be replaced by a refresh; look it up each time.
	pump.until(t, "the contents of "+path, func() bool {
		f := find()
		return f != nil && f.loaded
	})
	return find()
}

// ghostFixture is queue.txt committed with 26 lines, then 29 ("Append
// three"); the caller changes the work tree after.
func ghostFixture(t *testing.T) (root, file string) {
	t.Helper()
	root = gitFixture(t, "ghost")
	file = filepath.Join(root, "queue.txt")
	writeFile(t, file, numberedLines(1, 26))
	gitRun(t, root, "add", "queue.txt")
	gitRun(t, root, "commit", "-q", "-m", "Add queue")
	writeFile(t, file, numberedLines(1, 29))
	gitRun(t, root, "commit", "-q", "-am", "Append three")
	return root, file
}

func wantSides(t *testing.T, f *gdFile, oldN, newN int) {
	t.Helper()
	if got := len(f.oldLines); got != oldN {
		t.Errorf("old side has %d lines, want %d", got, oldN)
	}
	if got := len(f.newLines); got != newN {
		t.Errorf("new side has %d lines, want %d", got, newN)
	}
	if g := f.gapBefore(len(f.hunks())); g.count != 0 {
		t.Errorf("%d unchanged lines trail the last hunk; the diff ends with it", g.count)
	}
}

// An append-at-EOF commit whose file the work tree has since grown: the
// commit's diff must end at the commit's last line, not continue with the
// work tree's.
func TestCommitReviewReadsThatCommitsContents(t *testing.T) {
	root, file := ghostFixture(t)
	// Later work, uncommitted: four more lines.
	writeFile(t, file, numberedLines(1, 33))

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) >= 5 })
	reviewAndWait(t, pump, sh, "Append three")
	wantSides(t, reviewedFile(t, pump, sh, "queue.txt"), 26, 29)
}

// "Compare to Local Changes" reads the commit against the work tree: the old
// side is the commit's, the new side the work tree's own lines.
func TestCompareReadsTheCommitAndTheWorkTree(t *testing.T) {
	root, file := ghostFixture(t)
	// HEAD has moved on from the compared commit.
	writeFile(t, file, numberedLines(1, 31))
	gitRun(t, root, "commit", "-q", "-am", "Append two more")
	writeFile(t, file, numberedLines(1, 33))

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) >= 6 })
	for _, cm := range sh.git.commits {
		if cm.Subject == "Append three" {
			sh.reviewCommitAs(cm.Hash, true)
		}
	}
	pump.until(t, "the compare snapshot", func() bool { return sh.git.commitSnap != nil })
	f := reviewedFile(t, pump, sh, "queue.txt")
	if len(f.oldLines) != 29 || len(f.newLines) != 33 {
		t.Errorf("compare reads %d old and %d new lines, want the commit's 29 and the work tree's 33", len(f.oldLines), len(f.newLines))
	}
}

// A stash reads as its own change, whatever the work tree holds after it.
func TestStashReviewReadsTheStashContents(t *testing.T) {
	root, file := ghostFixture(t)
	writeFile(t, file, numberedLines(1, 31))
	gitRun(t, root, "stash", "push", "-q", "--", "queue.txt")
	writeFile(t, file, numberedLines(1, 40))

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "the stash", func() bool { return len(sh.git.stashes) == 1 })
	sh.reviewCommit(sh.git.stashes[0].Hash)
	pump.until(t, "the stash snapshot", func() bool { return sh.git.commitSnap != nil })
	wantSides(t, reviewedFile(t, pump, sh, "queue.txt"), 29, 31)
}

// A renamed file reads its old side under the old name at the parent and its
// new side under the new name at the commit.
func TestCommitReviewReadsARenameUnderBothNames(t *testing.T) {
	root, file := ghostFixture(t)
	gitRun(t, root, "mv", "queue.txt", "jobs.txt")
	moved := filepath.Join(root, "jobs.txt")
	writeFile(t, moved, numberedLines(1, 30))
	gitRun(t, root, "add", "jobs.txt")
	gitRun(t, root, "commit", "-q", "-m", "Rename queue")
	writeFile(t, moved, numberedLines(1, 36))
	_ = file

	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) >= 6 })
	reviewAndWait(t, pump, sh, "Rename queue")
	f := reviewedFile(t, pump, sh, "jobs.txt")
	if f.cf.OldPath != "queue.txt" {
		t.Fatalf("not a rename: old path %q", f.cf.OldPath)
	}
	wantSides(t, f, 29, 30)
}

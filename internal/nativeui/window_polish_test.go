package nativeui

/**
 * [INPUT]: window.go、frame.go / git_sidebar_tree.go 的侧栏、git_service_snapshot.go 的快照应用、gd_surface.go 的文件头、真实的临时仓库、headless ui.Tester
 * [OUTPUT]: 2026-11-08 打磨的合同：首读之前侧栏说「Reading references…」而不是五个 0 计数；外部改动不再冒出「Files changed on disk」提示、窗口激活直接读新数据；窄卡的文件头不画「Viewed」框，右键菜单保留它；提交列表的计数按单复数说「1 commit」而不是「1 commits」；欢迎页的两句话各占一行、没有孤字
 * [POS]: Solco Git 观察项 2 / 3 / 6 与体验巡检的回归防线；后台结果经 uiPump，测试线程不排空就能渲染「读取之前」的状态
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A workspace that has not read its references yet has nothing to count:
// five sections at "0" that a beat later fill read as a bug (2026-11-08).
func TestSidebarSaysItIsReadingBeforeItCounts(t *testing.T) {
	root := gitFixture(t, "reads")
	w, pump := newTestWindow(t)
	w.OpenRepository(root, viewCommits, "")
	tt := newTester(w, 1280, 800, true)
	tt.Frame() // no drain: the read has not landed
	if !tt.HasText("Reading references…") {
		t.Fatalf("before the first read the sidebar should say it is reading; texts: %q", tt.Texts())
	}
	for _, zero := range []string{"Worktrees", "Branches", "Stashes", "Tags", "Remotes"} {
		if tt.HasText(zero) {
			t.Errorf("section %q is listed (at 0) before anything was read", zero)
		}
	}
	sh := w.activeShell()
	pump.until(t, "the references", func() bool { return sh.git.refsRead })
	settle(tt, pump, 3)
	if tt.HasText("Reading references…") {
		t.Error("the reading note stays after the references landed")
	}
	if !tt.HasText("Branches") {
		t.Errorf("after the read the sidebar lists its sections; texts: %q", tt.Texts())
	}
}

// Every view that shows the changes re-reads them itself, so a different
// signature is not news worth a banner: the fresh data is the notification.
func TestExternalChangesShowWithoutAChangedOnDiskPill(t *testing.T) {
	root := gitFixture(t, "drift")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	before := sh.git.snapshot
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n\nChanged behind the app's back.\n")
	writeFile(t, filepath.Join(root, "extra.txt"), "new\n")
	sh.git.lastActivationRefresh = time.Time{}
	sh.refreshGitOnActivation() // the window became key
	pump.until(t, "the fresh snapshot", func() bool { return sh.git.snapshot != before })
	settle(tt, pump, 3)
	if tt.HasText("Files changed on disk") || tt.HasText("Refresh to update.") {
		t.Errorf("a changed-on-disk pill shows over data already updated; texts: %q", tt.Texts())
	}
	if _, ok := sh.git.snapshot.ByPath("extra.txt"); !ok {
		t.Error("the activation read did not pick the new file up")
	}
	if !tt.HasText("extra.txt") {
		t.Errorf("the new file is not listed; texts: %q", tt.Texts())
	}
}

// A bare box in a card header reads as a stage checkbox; Viewed says its word
// or is left to the header's menu.
func TestViewedIsAWordedPillOnlyWhereThereIsRoom(t *testing.T) {
	root := gitFixture(t, "viewed")
	w, pump := newTestWindow(t)
	openAndLoad(t, w, pump, root)
	roomy := newTester(w, 1280, 800, true)
	settle(roomy, pump, 4)
	if !roomy.HasText("Viewed") {
		t.Fatalf("a roomy card shows the Viewed pill; texts: %q", roomy.Texts())
	}
	narrow := newTester(w, 640, 800, true) // the center is far under gdHeaderRoomy
	settle(narrow, pump, 4)
	if narrow.HasText("Viewed") {
		t.Errorf("a narrow card must not draw a Viewed box; texts: %q", narrow.Texts())
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
}

// "1 commits" read as a typo above a one-row list (2026-11-08).
func TestCommitCountAgreesWithItsNumber(t *testing.T) {
	root := gitFixture(t, "count") // three commits
	one := filepath.Join(t.TempDir(), "one")
	if err := os.MkdirAll(one, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, one, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(one, "a.txt"), "a\n")
	gitRun(t, one, "add", ".")
	gitRun(t, one, "commit", "-q", "-m", "only")
	if resolved, err := filepath.EvalSymlinks(one); err == nil {
		one = resolved
	}
	w, pump := newTestWindow(t)
	w.OpenRepository(root, viewCommits, "")
	w.OpenRepository(one, viewCommits, "")
	tt := newTester(w, 1280, 800, true)
	for _, c := range []struct{ root, want, not string }{
		{one, "1 commit", "1 commits"},
		{root, "3 commits", ""},
	} {
		w.OpenRepository(c.root, viewCommits, "")
		sh := w.activeShell()
		pump.until(t, "the commits of "+filepath.Base(c.root), func() bool { return len(sh.git.commits) > 0 })
		settle(tt, pump, 3)
		if !tt.HasText(c.want) {
			t.Errorf("%s: no %q above the list; texts: %q", filepath.Base(c.root), c.want, tt.Texts())
		}
		if c.not != "" && tt.HasText(c.not) {
			t.Errorf("%s: %q is shown", filepath.Base(c.root), c.not)
		}
	}
}

// One sentence wrapped to a second line left the orphan word "workspace." under
// the centered title (2026-11-08): the welcome copy is two sentences, one line each.
func TestWelcomeCopyHasNoOrphanLine(t *testing.T) {
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	for _, line := range []string{
		"Review changes, browse history, read source.",
		"Each repository you open becomes a workspace.",
	} {
		r, ok := tt.Find(line)
		if !ok {
			t.Fatalf("the welcome page has no line %q; it shows %q", line, tt.Texts())
		}
		if r.H > 24 {
			t.Errorf("%q wraps onto %.0f px of height; each sentence must fit one line", line, r.H)
		}
	}
}

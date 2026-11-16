package nativeui

/**
 * [INPUT]: window.go 的焦点入口、shell_tab.go 的 pollGit、git_service_snapshot.go 的激活刷新与 loadBranches、right_panel_files.go 的静默刷新、真实的临时仓库
 * [OUTPUT]: 2026-11-16 的合同：在终端里发生的事（新分支、新提交、新文件）不必按 ⌘R 就会出现——窗口拿到焦点时、以及窗口开着的每个节拍里，快照、引用与提交图、Files 目录一并重读；Files 的重读不先清空屏上的内容
 * [POS]: 「有时必须手动刷新才看到最新数据」的回归防线；真实窗口的 OnFocus 由 AttachWindow 接线，这里测它调用的 onWindowFocus
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"testing"
	"time"
)

func hasBranch(sh *Shell, name string) bool {
	for _, b := range sh.git.branches {
		if b.Name == name {
			return true
		}
	}
	return false
}

// A window that gains focus reads again — the snapshot and the references.
func TestWindowFocusRereadsWhatHappenedOutside(t *testing.T) {
	root := gitFixture(t, "focus")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the first read of the references", func() bool { return sh.git.refsRead })
	before := sh.git.snapshot

	gitRun(t, root, "branch", "made-in-terminal")
	gitRun(t, root, "commit", "-q", "--allow-empty", "-m", "Committed from a terminal")
	writeFile(t, filepath.Join(root, "from-terminal.txt"), "hi\n")

	sh.git.lastActivationRefresh = time.Time{}
	w.onWindowFocus()

	pump.until(t, "the snapshot after the focus", func() bool {
		if sh.git.snapshot == before {
			return false
		}
		_, ok := sh.git.snapshot.ByPath("from-terminal.txt")
		return ok
	})
	pump.until(t, "the references after the focus", func() bool {
		return hasBranch(sh, "made-in-terminal") &&
			len(sh.git.commits) > 0 && sh.git.commits[0].Subject == "Committed from a terminal"
	})
}

// Focus handed back and forth between a dialog and the window is not an
// absence: the second activation inside the gap reads nothing.
func TestFocusBounceWithinTheGapReadsOnce(t *testing.T) {
	root := gitFixture(t, "bounce")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the first read of the references", func() bool { return sh.git.refsRead })
	sh.git.lastActivationRefresh = time.Time{}
	w.onWindowFocus()
	pump.drain()
	gen := sh.git.branchesGen.Load()
	w.onWindowFocus()
	if got := sh.git.branchesGen.Load(); got != gen {
		t.Errorf("a second focus inside the gap started another read (generation %d -> %d)", gen, got)
	}
}

// With the window left open and nothing clicked, the beat re-reads the
// references on its own — not only the snapshot.
func TestThePollRereadsReferencesOnItsOwnBeat(t *testing.T) {
	root := gitFixture(t, "beat")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the first read of the references", func() bool { return sh.git.refsRead })

	gitRun(t, root, "branch", "later")
	clock := time.Now()
	timeNow = func() time.Time { return clock }
	t.Cleanup(func() { timeNow = time.Now })

	gen := sh.git.branchesGen.Load()
	clock = clock.Add(refsPollEvery / 2)
	sh.pollGit()
	if got := sh.git.branchesGen.Load(); got != gen {
		t.Fatalf("the beat re-read the references before %s had passed", refsPollEvery)
	}
	clock = clock.Add(refsPollEvery)
	sh.pollGit()
	pump.until(t, "the references after the beat", func() bool { return hasBranch(sh, "later") })
}

// A running Git operation owns the references: neither beat nor focus starts
// a competing read over it.
func TestTheBeatLeavesABusyWorkspaceAlone(t *testing.T) {
	root := gitFixture(t, "busy")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the first read of the references", func() bool { return sh.git.refsRead })
	sh.git.opBusy = true
	sh.git.lastRefsPoll = time.Time{}
	gen := sh.git.branchesGen.Load()
	sh.pollGit()
	sh.git.lastActivationRefresh = time.Time{}
	sh.refreshGitOnActivation()
	if got := sh.git.branchesGen.Load(); got != gen {
		t.Errorf("a read of the references started over a running operation (generation %d -> %d)", gen, got)
	}
}

// Files: a file made outside shows after the focus, and the re-read never
// empties the listing first (a blank tree for a beat reads as a flicker).
func TestFocusRelistsTheFilesTreeWithoutBlankingIt(t *testing.T) {
	root := gitFixture(t, "tree")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.setView(viewFiles)
	tt := newTester(w, 1280, 800, true)
	settle(tt, pump, 3)
	st := &sh.rightPanel
	if len(st.cachedDirEntries[st.currentRoot]) == 0 {
		t.Fatalf("the Files root was not listed before the check; cache: %v", st.cachedDirEntries)
	}
	gen := st.filesGen.Load()

	writeFile(t, filepath.Join(root, "made-outside.txt"), "x\n")
	sh.git.lastActivationRefresh = time.Time{}
	w.onWindowFocus()

	if st.filesGen.Load() != gen {
		t.Error("the focus re-read bumped the files generation: that is the hard refresh, which blanks the tree")
	}
	if len(st.cachedDirEntries[st.currentRoot]) == 0 {
		t.Error("the tree was emptied while it re-read")
	}
	pump.until(t, "the new file in the listing", func() bool {
		for _, e := range st.cachedDirEntries[st.currentRoot] {
			if e.Name == "made-outside.txt" {
				return true
			}
		}
		return false
	})
}

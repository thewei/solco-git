package nativeui

/**
 * [INPUT]: window.go OpenRepository 的链接入口、gd_surface.go 的 diff 列表、真实的临时仓库、headless ui.Tester
 * [OUTPUT]: 冷启动链接落点的合同：Solco 经 solcogit:// 链接首次读仓库时，Changes 列表落在被点名文件的卡片上，而不是偏上的几张卡
 * [POS]: 2026-11-08 观察项 1 的回归防线：卡片还没读到之前链接就到了，首读落地后的滚动要等卡片高度稳定
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// landingFixture is a repository whose twelve changed files are each a long
// file with two small edits, so every card has hidden unchanged lines around
// its hunks and the list of cards is far taller than the viewport.
func landingFixture(t *testing.T) (root string, paths []string) {
	t.Helper()
	root = gitFixture(t, "landing")
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "Baseline")
	body := func(edit bool) string {
		var b strings.Builder
		for i := 1; i <= 400; i++ {
			switch {
			case edit && (i >= 100 && i <= 110 || i >= 250 && i <= 255):
				fmt.Fprintf(&b, "line %03d edited\n", i)
			default:
				fmt.Fprintf(&b, "line %03d\n", i)
			}
		}
		return b.String()
	}
	for i := 1; i <= 12; i++ {
		p := fmt.Sprintf("pad/f%02d.txt", i)
		paths = append(paths, p)
		writeFile(t, filepath.Join(root, p), body(false))
	}
	gitRun(t, root, "add", "-A")
	gitRun(t, root, "commit", "-q", "-m", "Add pad")
	for _, p := range paths {
		writeFile(t, filepath.Join(root, p), body(true))
	}
	return root, paths
}

// topCard is the file whose card the list shows at its top.
func topCard(sh *Shell) (path string, first int) {
	first, _ = sh.git.gdList.Visible()
	if first < 0 || first >= len(sh.git.gdRows) {
		return "", first
	}
	return sh.git.gdFiles[sh.git.gdRows[first].file].cf.Path, first
}

func TestColdLinkLandsOnTheNamedCard(t *testing.T) {
	for _, idx := range []int{0, 3, 8, 11} {
		t.Run(fmt.Sprintf("file-%02d", idx+1), func(t *testing.T) {
			root, paths := landingFixture(t)
			target := paths[idx]
			clock := time.Now()
			timeNow = func() time.Time { return clock }
			t.Cleanup(func() { timeNow = time.Now })
			w, pump := newTestWindow(t)
			tt := newTester(w, 1280, 800, true)
			w.OpenRepository(root, viewChanges, target) // a link: the window has no read yet
			sh := w.activeShell()
			// Frames run while the read is on its way, as they do in the app.
			wait := func(what string, done func() bool) {
				t.Helper()
				for deadline := time.Now().Add(20 * time.Second); !done(); {
					if time.Now().After(deadline) {
						t.Fatalf("timed out waiting for %s", what)
					}
					pump.drain()
					tt.Frame()
					time.Sleep(2 * time.Millisecond)
				}
			}
			wait("the first read and the reveal", func() bool { return sh.git.snapshot != nil && sh.git.revealPath == target })
			assertLanded := func(when string) {
				t.Helper()
				got, first := topCard(sh)
				if got != target {
					t.Fatalf("%s: the list's top card is %q (row %d), want %q; selected %q",
						when, got, first, target, sh.surface.diff.SelectedPath)
				}
			}
			assertLanded("right after the first read")
			// The cards' contents arrive in batches after the landing and add
			// rows above it; the card must stay where the link put it.
			wait("every card's contents", func() bool {
				for _, f := range sh.git.gdFiles {
					if !f.loaded {
						return false
					}
				}
				return true
			})
			settle(tt, pump, 3)
			assertLanded("after the contents loaded")
			// Past the reveal window the selection follows the scroll again:
			// it must still be the linked file.
			clock = clock.Add(2 * time.Second)
			settle(tt, pump, 3)
			assertLanded("after the reveal window")
			if got := sh.surface.diff.SelectedPath; got != target {
				t.Errorf("selected = %q after the reveal window, want %q", got, target)
			}
			saveEvidence(t, "cold-link-landing", tt)
		})
	}
}

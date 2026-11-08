package nativeui

/**
 * [INPUT]: window.go / frame.go / welcome.go / quick_launch.go / window_workspaces.go、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 窗口渲染合同：欢迎页与仓库工作区页都能完整渲染（含工作区下拉与设置里的工作区管理）；设置 SOLCOGIT_EVIDENCE 时落设计对照用的 PNG
 * [POS]: 与设计稿 designs/git-files-app 逐屏对照的证据来源（无需前台窗口，锁屏时也能取证）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"testing"
)

func TestWindowRendersEvidence(t *testing.T) {
	for _, dark := range []bool{true, false} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		w, pump := newTestWindow(t)
		tt := newTester(w, 1280, 800, dark)
		settle(tt, pump, 3)
		saveEvidence(t, "welcome-"+mode, tt)

		app := gitFixture(t, "app")
		docs := gitFixture(t, "docs")
		openAndLoad(t, w, pump, docs)
		sh := openAndLoad(t, w, pump, app)
		pump.until(t, "the branch list", func() bool { return len(sh.git.branches) > 0 && !sh.git.branchesLoading })
		settle(tt, pump, 4)
		saveEvidence(t, "changes-"+mode, tt)

		sh.showView(viewCommits)
		pump.until(t, "the commit list", func() bool { return len(sh.git.commits) > 0 })
		settle(tt, pump, 4)
		saveEvidence(t, "commits-"+mode, tt)

		sh.showView(viewFiles)
		settle(tt, pump, 4)
		saveEvidence(t, "files-"+mode, tt)

		if err := tt.Click("Workspace: app"); err != nil { // the way a user opens it
			t.Fatal(err)
		}
		settle(tt, pump, 3)
		saveEvidence(t, "workspaces-"+mode, tt)
		w.picker.open = false

		w.openQuickLaunch()
		settle(tt, pump, 3)
		saveEvidence(t, "quick-"+mode, tt)
		w.quick.open = false

		w.openSettings()
		settle(tt, pump, 3)
		saveEvidence(t, "settings-"+mode, tt)
		w.settingsOpen = false

		// Removing every workspace ends at the welcome page, now with the recents.
		w.removeWorkspace(1)
		w.removeWorkspace(0)
		settle(tt, pump, 2)
		pump.until(t, "the recent branches", func() bool { return len(w.recentBranch) == 2 })
		settle(tt, pump, 3)
		saveEvidence(t, "welcome-recent-"+mode, tt)
	}
}

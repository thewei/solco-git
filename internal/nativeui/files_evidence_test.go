package nativeui

/**
 * [INPUT]: files_git_test.go 的夹具、headless ui.Tester
 * [OUTPUT]: 设置 SOLCOGIT_EVIDENCE 时落下 Files 视图的 PNG（Changed 视图、槽边改动条、Blame、未跟踪文件），不设置时只跑一遍渲染不报错
 * [POS]: 没有真实窗口时的视觉证据；断言放在 files_git_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"testing"
)

func TestFilesEvidence(t *testing.T) {
	for _, dark := range []bool{true, false} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		w, pump, sh, root := filesFixtureShell(t)
		tt := newTester(w, 1280, 800, dark)
		settle(tt, pump, 3)
		saveEvidence(t, "files-start-"+mode, tt)

		sh.openFilePreview(filepath.Join(root, "README.md"))
		settle(tt, pump, 3)
		saveEvidence(t, "files-all-gutter-"+mode, tt)

		sh.filesGit.changedOnly = true
		sh.openFilePreview(filepath.Join(root, "src", "lib", "util.go"))
		settle(tt, pump, 3)
		saveEvidence(t, "files-changed-"+mode, tt)

		sh.openFilePreview(filepath.Join(root, "notes.txt"))
		settle(tt, pump, 3)
		saveEvidence(t, "files-untracked-"+mode, tt)

		sh.filesGit.changedOnly = false
		sh.openFilePreview(filepath.Join(root, "README.md"))
		settle(tt, pump, 2)
		if err := tt.Click("Blame"); err != nil {
			t.Fatal(err)
		}
		pump.until(t, "blame", func() bool { return !sh.filesGit.blameLoading && len(sh.filesGit.blameLines) > 0 })
		settle(tt, pump, 3)
		saveEvidence(t, "files-blame-"+mode, tt)
	}
}

func TestCommitTreeEvidence(t *testing.T) {
	for _, dark := range []bool{true, false} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		w, pump, sh, _ := treeFixture(t)
		tt := newTester(w, 1280, 800, dark)
		reviewAndWait(t, pump, sh, "Add main")
		settle(tt, pump, 3)
		saveEvidence(t, "commit-changes-"+mode, tt)
		if err := tt.Click("Tree"); err != nil {
			t.Fatal(err)
		}
		pump.until(t, "root", func() bool { return sh.commitTree.dirs[""] != nil })
		settle(tt, pump, 3)
		if err := tt.Click("src"); err != nil {
			t.Fatal(err)
		}
		pump.until(t, "src", func() bool { return sh.commitTree.dirs["src"] != nil })
		settle(tt, pump, 3)
		if err := tt.Click("main.go"); err != nil {
			t.Fatal(err)
		}
		pump.until(t, "main.go", func() bool { return sh.commitTree.file.code != nil })
		settle(tt, pump, 3)
		saveEvidence(t, "commit-tree-"+mode, tt)
	}
}

func TestPushDialogEvidence(t *testing.T) {
	for _, dark := range []bool{true, false} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		w, pump, sh, _ := pushShell(t)
		tt := newTester(w, 1280, 800, dark)
		settle(tt, pump, 2)
		sh.openMultiPush()
		d := &sh.gitMore.push
		pump.until(t, "the offer", func() bool { return !d.loading && len(d.branches) > 0 })
		d.picked["t:v1"] = true
		settle(tt, pump, 3)
		saveEvidence(t, "push-dialog-"+mode, tt)
	}
}

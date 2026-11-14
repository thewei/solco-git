package nativeui

/**
 * [INPUT]: changes_panel.go / gd_surface.go 的行菜单、git_operations.go 的 ignorePaths、真实临时仓库
 * [OUTPUT]: “忽略文件”的界面合同：未跟踪文件的右键菜单有 Add to .gitignore 与 Add to .git/info/exclude，选了就写对应文件、文件从变更列表消失；已跟踪文件没有这两项
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/ignore_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestUntrackedRowOffersGitignoreAndExclude(t *testing.T) {
	root := gitFixture(t, "ignore-menu")
	writeFile(t, filepath.Join(root, "scratch.log"), "noise\n")
	writeFile(t, filepath.Join(root, "local.env"), "SECRET=1\n")
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n\nedited\n")
	w, pump := newTestWindow(t)
	tt := newTester(w, 1280, 800, true)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "the changes", func() bool { return len(sh.git.snapshot.Files) >= 3 })
	settle(tt, pump, 3)
	has := func(rel string) bool {
		for _, f := range sh.git.snapshot.Files {
			if f.Path == rel {
				return true
			}
		}
		return false
	}

	// A tracked, edited file: no ignore entries.
	if err := tt.RightClick("README.md"); err != nil {
		t.Fatalf("no row: %v (%q)", err, tt.Texts())
	}
	for _, item := range tt.Menu() {
		if strings.HasPrefix(item, "Add to .") {
			t.Fatalf("a tracked file offers %q: ignoring it would not untrack it", item)
		}
	}
	tt.Key(0, ui.KeyEscape)

	if err := tt.RightClick("scratch.log"); err != nil {
		t.Fatalf("no row: %v (%q)", err, tt.Texts())
	}
	menu := tt.Menu()
	if !slices.Contains(menu, "Add to .gitignore") || !slices.Contains(menu, "Add to .git/info/exclude") {
		t.Fatalf("untracked row menu = %q", menu)
	}
	if err := tt.ChooseMenuItem("Add to .gitignore"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the file leaves the list", func() bool { return !sh.git.opBusy && !has("scratch.log") })
	if data, _ := os.ReadFile(filepath.Join(root, ".gitignore")); !strings.Contains(string(data), "/scratch.log\n") {
		t.Errorf(".gitignore = %q", data)
	}

	settle(tt, pump, 2)
	if err := tt.RightClick("local.env"); err != nil {
		t.Fatalf("no row: %v (%q)", err, tt.Texts())
	}
	if err := tt.ChooseMenuItem("Add to .git/info/exclude"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the second file leaves", func() bool { return !sh.git.opBusy && !has("local.env") })
	if data, _ := os.ReadFile(filepath.Join(root, ".git", "info", "exclude")); !strings.Contains(string(data), "/local.env\n") {
		t.Errorf("info/exclude = %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(root, ".gitignore")); strings.Contains(string(data), "local.env") {
		t.Errorf("a local exclude must not touch .gitignore: %q", data)
	}
}

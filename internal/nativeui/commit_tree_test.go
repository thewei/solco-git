package nativeui

/**
 * [INPUT]: commit_tree.go、真实临时仓库（含一个二进制文件的提交）、headless ui.Tester
 * [OUTPUT]: Tree 模式的合同：切到 Tree 才列该提交那一刻的整棵树、目录按需展开、文件只读展示那一刻的内容并标出"At <hash>"、二进制与不存在的文件给出说明而非乱码、换提交即换树
 * [POS]: 2026-11-07「Solco Git」决定里 Fork File Tree（按提交看整棵树）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// treeFixture is the Files fixture plus a commit that adds a binary file.
func treeFixture(t *testing.T) (*Window, *uiPump, *Shell, string) {
	t.Helper()
	root := gitFixture(t, "tree")
	gitRun(t, root, "stash", "-u", "-q")
	if err := os.WriteFile(filepath.Join(root, "logo.bin"), []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", "logo.bin")
	gitRun(t, root, "commit", "-q", "-m", "Add logo")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewCommits)
	pump.until(t, "commits", func() bool { return len(sh.git.commits) == 4 })
	return w, pump, sh, root
}

// reviewAndWait reviews the commit with the given subject and waits for its
// metadata.
func reviewAndWait(t *testing.T, pump *uiPump, sh *Shell, subject string) string {
	t.Helper()
	for _, cm := range sh.git.commits {
		if cm.Subject == subject {
			sh.reviewCommit(cm.Hash)
			pump.until(t, "review of "+subject, func() bool {
				return sh.git.commitMeta != nil && sh.git.commitMeta.Hash == cm.Hash && sh.git.commitSnap != nil
			})
			return cm.Hash
		}
	}
	t.Fatalf("no commit %q in %d commits", subject, len(sh.git.commits))
	return ""
}

func TestTreeModeListsTheTreeAtThatCommit(t *testing.T) {
	w, pump, sh, _ := treeFixture(t)
	tt := newTester(w, 1280, 800, true)
	reviewAndWait(t, pump, sh, "Add main")
	settle(tt, pump, 3)
	if sh.treeAt() {
		t.Fatal("Changes is the default tab")
	}
	if !tt.HasText("Changes") || !tt.HasText("Tree") {
		t.Fatalf("the Changes | Tree switch is missing; texts: %q", tt.Texts())
	}
	if err := tt.Click("Tree"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the root listing", func() bool { return sh.commitTree.dirs[""] != nil })
	settle(tt, pump, 3)
	if !sh.treeAt() {
		t.Fatal("Tree mode did not engage")
	}
	for _, want := range []string{"README.md", "src"} {
		if !tt.HasText(want) {
			t.Errorf("tree at Add main lacks %q; texts: %q", want, tt.Texts())
		}
	}
	if tt.HasText("logo.bin") {
		t.Error("logo.bin was added by a later commit")
	}
	// A folder lists on demand.
	if err := tt.Click("src"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "src listing", func() bool { return sh.commitTree.dirs["src"] != nil })
	settle(tt, pump, 3)
	for _, want := range []string{"main.go", "lib"} {
		if !tt.HasText(want) {
			t.Errorf("src lacks %q; texts: %q", want, tt.Texts())
		}
	}
	// A file opens read-only, as it stood.
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "main.go", func() bool { return sh.commitTree.file.code != nil })
	settle(tt, pump, 3)
	code := sh.commitTree.file.code
	if got := code.Buffer.String(); got != "package main\n\nfunc main() {}\n" {
		t.Errorf("main.go at that commit = %q", got)
	}
	if !code.ReadOnly {
		t.Error("a file read from a commit is read-only")
	}
	short := sh.git.commitMeta.Short
	// (The editor paints its own text, so the Tester's text list holds the
	// header and the foot but not the lines.)
	if !tt.HasText("At "+short) || !tt.HasText("Go · UTF-8 · 3 lines") {
		t.Errorf("the file view lacks its commit chip or foot; texts: %q", tt.Texts())
	}
}

func TestTreeModeSaysWhyThereIsNoCode(t *testing.T) {
	w, pump, sh, _ := treeFixture(t)
	tt := newTester(w, 1280, 800, true)
	reviewAndWait(t, pump, sh, "Add logo")
	sh.commitTree.on = true
	settle(tt, pump, 2)
	pump.until(t, "the root listing", func() bool { return sh.commitTree.dirs[""] != nil })
	settle(tt, pump, 3)
	if err := tt.Click("logo.bin"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "logo.bin", func() bool { return !sh.commitTree.file.loading })
	settle(tt, pump, 3)
	if sh.commitTree.file.code != nil || sh.commitTree.file.note != "Binary file" {
		t.Errorf("binary file state = %+v", sh.commitTree.file)
	}
	if !tt.HasText("Binary file") {
		t.Errorf("the pane should say Binary file; texts: %q", tt.Texts())
	}
}

func TestTreeModeFollowsTheCommitUnderReview(t *testing.T) {
	w, pump, sh, _ := treeFixture(t)
	tt := newTester(w, 1280, 800, true)
	sh.commitTree.on = true
	reviewAndWait(t, pump, sh, "Add logo")
	settle(tt, pump, 2)
	pump.until(t, "root listing", func() bool { return sh.commitTree.dirs[""] != nil })
	settle(tt, pump, 3)
	if !tt.HasText("logo.bin") {
		t.Fatalf("the tip has logo.bin; texts: %q", tt.Texts())
	}
	reviewAndWait(t, pump, sh, "Initial commit")
	settle(tt, pump, 2)
	pump.until(t, "root listing of the first commit", func() bool {
		return sh.commitTree.dirs[""] != nil && sh.commitTree.hash == sh.git.commitMeta.Hash
	})
	settle(tt, pump, 3)
	if tt.HasText("logo.bin") || strings.Contains(strings.Join(tt.Texts(), " "), "main.go") {
		t.Errorf("the first commit has neither logo.bin nor main.go; texts: %q", tt.Texts())
	}
	if !tt.HasText("README.md") || !sh.commitTree.on {
		t.Error("the chosen tab stays and the tree is the first commit's")
	}
	if sh.commitTree.selected != "" {
		t.Error("another commit starts with no file open")
	}
}

package nativeui

/**
 * [INPUT]: multi_push.go、带本地裸远端的临时仓库、headless ui.Tester
 * [OUTPUT]: Push 对话框的合同：列出有东西可推的分支（当前分支预选、新分支、分叉受阻）与本地标签，足迹行数字随勾选变化，Push 只推勾选的引用且不强推，没有远端时不开对话框
 * [POS]: 2026-11-07「Solco Git」决定里多引用推送（Fork 的 Push 对话框）的回归防线
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// remoteFixture is a repository whose origin is a local bare repository:
// main tracks origin/main and is one commit ahead, feature is new, diverged is
// one ahead and one behind, and v1 is a local tag.
func remoteFixture(t *testing.T) (root, bare string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	base := t.TempDir()
	bare = filepath.Join(base, "origin.git")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, bare, "init", "-q", "--bare", "-b", "main")
	root = filepath.Join(base, "work")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-q", "-b", "main")
	gitRun(t, root, "remote", "add", "origin", bare)
	writeFile(t, filepath.Join(root, "a.txt"), "one\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "one")
	gitRun(t, root, "push", "-q", "-u", "origin", "main")
	gitRun(t, root, "branch", "diverged")
	gitRun(t, root, "push", "-q", "-u", "origin", "diverged")
	gitRun(t, root, "checkout", "-q", "diverged")
	writeFile(t, filepath.Join(root, "d.txt"), "local\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "local on diverged")
	other := filepath.Join(base, "other")
	gitRun(t, base, "clone", "-q", bare, other)
	gitRun(t, other, "checkout", "-q", "diverged")
	writeFile(t, filepath.Join(other, "r.txt"), "remote\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "-c", "user.name=Other", "-c", "user.email=o@example.com", "commit", "-q", "-m", "remote on diverged")
	gitRun(t, other, "push", "-q", "origin", "diverged")
	gitRun(t, root, "fetch", "-q", "origin")
	gitRun(t, root, "checkout", "-q", "-b", "feature", "main")
	writeFile(t, filepath.Join(root, "f.txt"), "feature\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "on feature")
	gitRun(t, root, "checkout", "-q", "main")
	writeFile(t, filepath.Join(root, "m.txt"), "ahead\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "ahead on main")
	gitRun(t, root, "tag", "v1")
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, bare
}

func pushShell(t *testing.T) (*Window, *uiPump, *Shell, string) {
	t.Helper()
	root, bare := remoteFixture(t)
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	pump.until(t, "remotes", func() bool { return len(sh.git.remotes) > 0 && sh.git.upstream.OK })
	return w, pump, sh, bare
}

func TestPushDialogOffersWhatThereIsToSend(t *testing.T) {
	w, pump, sh, _ := pushShell(t)
	tt := newTester(w, 1280, 800, true)
	sh.openMultiPush()
	d := &sh.gitMore.push
	pump.until(t, "the offer", func() bool { return !d.loading && (len(d.branches) > 0 || d.err != "") })
	if d.err != "" {
		t.Fatalf("offer failed: %s", d.err)
	}
	settle(tt, pump, 3)
	for _, want := range []string{"Push to origin", "main", "current", "1 commit to send", "feature", "new on origin",
		"diverged", "pull first", "v1", "1 branch · 0 tags · no force push", "Push 1 ref"} {
		if !tt.HasText(want) {
			t.Errorf("dialog lacks %q; texts: %q", want, tt.Texts())
		}
	}
	// The diverged branch can never be chosen.
	d.picked["b:diverged"] = true
	if refs, _, _ := d.chosenRefs(); len(refs) != 1 || refs[0].Name != "main" {
		t.Errorf("chosen = %+v, want just main", refs)
	}
}

func TestPushDialogPushesTheChosenRefs(t *testing.T) {
	w, pump, sh, bare := pushShell(t)
	tt := newTester(w, 1280, 800, true)
	sh.openMultiPush()
	d := &sh.gitMore.push
	pump.until(t, "the offer", func() bool { return !d.loading && len(d.branches) > 0 })
	d.picked["b:feature"] = true
	d.picked["t:v1"] = true
	settle(tt, pump, 2)
	if !tt.HasText("2 branches · 1 tag · no force push") || !tt.HasText("Push 3 refs") {
		t.Fatalf("footer did not follow the choice; texts: %q", tt.Texts())
	}
	if err := tt.Click("Push 3 refs"); err != nil {
		t.Fatal(err)
	}
	pump.until(t, "the push to finish", func() bool { return !sh.git.opBusy && !d.open })
	settle(tt, pump, 2)
	refs := gitRun(t, bare, "for-each-ref", "--format=%(refname)")
	for _, want := range []string{"refs/heads/main", "refs/heads/feature", "refs/tags/v1"} {
		if !strings.Contains(refs, want) {
			t.Errorf("origin lacks %s:\n%s", want, refs)
		}
	}
	local := gitRun(t, filepath.Dir(bare)+"/work", "rev-parse", "main")
	if remote := gitRun(t, bare, "rev-parse", "main"); remote != local {
		t.Errorf("origin/main = %s, local main = %s", remote, local)
	}
}

func TestPushDialogNeedsARemote(t *testing.T) {
	root := gitFixture(t, "norem")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.openMultiPush()
	if sh.gitMore.push.open {
		t.Error("with no remote there is nothing to open")
	}
	if sh.pendingToast == "" {
		t.Error("say why nothing opened")
	}
}

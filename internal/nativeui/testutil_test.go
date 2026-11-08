package nativeui

/**
 * [INPUT]: window.go / shell.go、真实的临时 Git 仓库、headless ui.Tester
 * [OUTPUT]: 测试公用件：gitFixture（带历史、分支、改动、嵌套目录的仓库）、newTestWindow（内存偏好 + 可排空的后台应用队列）、uiPump.until（等后台结果落到 UI 线）、frames（渲染若干帧）、saveEvidence（设置 SOLCOGIT_EVIDENCE 时落 PNG）
 * [POS]: 本包所有 *_test.go 共享；后台结果不经窗口 Update 车道而进队列，由测试线程排空，保证无竞态、可确定
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/thewei/solco-git/internal/settings"
)

// gitRun runs git in dir with a fixed identity and returns its output.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gitFixture is a repository named name with three commits on main, a branch
// "feature", a modified README.md, an untracked notes.txt and a nested
// src/lib/util.go. The returned path is symlink-resolved, as Git reports it.
func gitFixture(t *testing.T, name string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n\nA small project.\n")
	writeFile(t, filepath.Join(root, "src", "lib", "util.go"), "package lib\n\nfunc Add(a, b int) int { return a + b }\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "Initial commit")
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main\n\nfunc main() {}\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "Add main")
	gitRun(t, root, "branch", "feature")
	writeFile(t, filepath.Join(root, "src", "lib", "util.go"), "package lib\n\n// Add sums two ints.\nfunc Add(a, b int) int { return a + b }\n")
	gitRun(t, root, "commit", "-q", "-am", "Document Add")
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n\nA small project.\n\nSecond paragraph.\n")
	writeFile(t, filepath.Join(root, "notes.txt"), "todo\n")
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root
}

// uiPump stands in for the window's update lane: background results queue
// here and the test goroutine runs them, so no frame ever races a result.
type uiPump struct {
	mu sync.Mutex
	q  []func()
}

func (p *uiPump) post(fn func()) {
	p.mu.Lock()
	p.q = append(p.q, fn)
	p.mu.Unlock()
}

// drain runs what is queued (and what that queues) and reports how many ran.
func (p *uiPump) drain() int {
	ran := 0
	for {
		p.mu.Lock()
		q := p.q
		p.q = nil
		p.mu.Unlock()
		if len(q) == 0 {
			return ran
		}
		for _, fn := range q {
			fn()
			ran++
		}
	}
}

// until pumps the lane until cond holds or the deadline passes.
func (p *uiPump) until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		p.drain()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// newTestWindow is a window on an in-memory store with no native window.
func newTestWindow(t *testing.T) (*Window, *uiPump) {
	t.Helper()
	pump := &uiPump{}
	w := NewWindow(settings.NewMemoryStore(settings.Default()))
	w.uiApply = pump.post
	return w, pump
}

// open opens a repository the production way and waits for its snapshot.
func openAndLoad(t *testing.T, w *Window, pump *uiPump, root string) *Shell {
	t.Helper()
	w.OpenRepository(root, "", "")
	i := w.indexOfRoot(root)
	if i < 0 {
		t.Fatalf("%s did not open", root)
	}
	sh := w.workspaces[i].sh
	pump.until(t, "the changes snapshot of "+filepath.Base(root), func() bool {
		return sh.git.snapshot != nil && sh.git.root == root
	})
	return sh
}

// newTester is a headless renderer of the window in the given appearance.
func newTester(w *Window, width, height int, dark bool) *ui.Tester {
	tt := ui.NewTester(w.View, width, height)
	tt.SetDark(dark)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	return tt
}

// settle renders n frames, running the background results between them, then
// lets the animations finish: the Tester's clock stands still, so a row that
// fades out (or a tab pill that slides) would otherwise stay painted mid-way.
func settle(tt *ui.Tester, pump *uiPump, n int) {
	for i := 0; i < n; i++ {
		pump.drain()
		tt.Frame()
	}
	tt.Advance(time.Second)
	pump.drain()
	tt.Frame()
}

// saveEvidence writes the rendered frame as a PNG under $SOLCOGIT_EVIDENCE
// (skipped when unset): the layout is read by eye against the design pages.
func saveEvidence(t *testing.T, name string, tt *ui.Tester) {
	t.Helper()
	dir := os.Getenv("SOLCOGIT_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

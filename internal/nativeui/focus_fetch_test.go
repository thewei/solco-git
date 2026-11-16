package nativeui

/**
 * [INPUT]: git_fork_more.go 的 quietFetch（自动抓取与「回到窗口时抓取」共用）、git_service_snapshot.go 的激活刷新、settings 的 NoFetchOnFocus、带裸远程的真实临时仓库（remoteFixture）
 * [OUTPUT]: 2026-11-16 的合同：窗口拿到焦点时，距上次 Fetch 超过 10 分钟就静默 fetch 一次（只 fetch，不合并不推送），远程上别人的提交随即出现在 Behind 里；刚抓过、设置里关掉、或上一次尝试还在 10 分钟之内都不再联网；失败（离线、要登录）静默
 * [POS]: 「远程变化也要自动感知」的回归防线；联网是产品决策，开关在 Settings → Git
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// otherPushes makes the second clone push one more commit to origin/main.
func otherPushes(t *testing.T, bare, file string) {
	t.Helper()
	other := filepath.Join(filepath.Dir(bare), "other")
	gitRun(t, other, "checkout", "-q", "main")
	gitRun(t, other, "pull", "-q", "--rebase", "origin", "main")
	writeFile(t, filepath.Join(other, file), "x\n")
	gitRun(t, other, "add", ".")
	gitRun(t, other, "-c", "user.name=Other", "-c", "user.email=o@example.com", "commit", "-q", "-m", "from elsewhere "+file)
	gitRun(t, other, "push", "-q", "origin", "main")
}

// ageFetch makes the last fetch look old (or new): FETCH_HEAD's mtime is
// what the Fetch button's "Fetched … ago" reads.
func ageFetch(t *testing.T, sh *Shell, ago time.Duration) {
	t.Helper()
	p := filepath.Join(sh.git.root, ".git", "FETCH_HEAD")
	at := time.Now().Add(-ago)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

func originMain(t *testing.T, sh *Shell) string {
	t.Helper()
	return gitRun(t, sh.git.root, "rev-parse", "refs/remotes/origin/main")
}

func focus(sh *Shell) {
	sh.git.lastActivationRefresh = time.Time{}
	sh.refreshGitOnActivation()
}

// idle lets background work land for a moment and reports nothing happened.
func idle(pump *uiPump) {
	for i := 0; i < 25; i++ {
		pump.drain()
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFocusFetchesWhenTheLastFetchIsOld(t *testing.T) {
	_, pump, sh, bare := pushShell(t)
	otherPushes(t, bare, "one.txt")
	ageFetch(t, sh, time.Hour)
	before := originMain(t, sh)
	focus(sh)
	pump.until(t, "the fetch after the focus", func() bool { return originMain(t, sh) != before })
	pump.until(t, "the references after the fetch", func() bool {
		return len(sh.git.commits) > 0 && sh.git.upstream.OK
	})
}

func TestFocusDoesNotFetchWhatWasJustFetched(t *testing.T) {
	_, pump, sh, bare := pushShell(t)
	otherPushes(t, bare, "two.txt")
	ageFetch(t, sh, time.Minute)
	before := originMain(t, sh)
	focus(sh)
	idle(pump)
	if got := originMain(t, sh); got != before {
		t.Error("a fetch from a minute ago was repeated on focus")
	}
}

func TestFocusFetchIsOffWhenTheSettingSaysSo(t *testing.T) {
	_, pump, sh, bare := pushShell(t)
	sh.settings.Workbench.NoFetchOnFocus = true
	otherPushes(t, bare, "three.txt")
	ageFetch(t, sh, time.Hour)
	before := originMain(t, sh)
	focus(sh)
	idle(pump)
	if got := originMain(t, sh); got != before {
		t.Error("focus fetched although Fetch on return is off")
	}
}

// An offline or signed-out remote must not be tried again at every focus.
func TestFocusFetchTriesOncePerStalePeriod(t *testing.T) {
	_, pump, sh, bare := pushShell(t)
	otherPushes(t, bare, "four.txt")
	ageFetch(t, sh, time.Hour)
	before := originMain(t, sh)
	focus(sh)
	pump.until(t, "the first fetch", func() bool { return originMain(t, sh) != before })
	idle(pump)

	otherPushes(t, bare, "five.txt")
	ageFetch(t, sh, time.Hour) // looks stale again, but the attempt was just made
	mid := originMain(t, sh)
	focus(sh)
	idle(pump)
	if got := originMain(t, sh); got != mid {
		t.Error("a second fetch started inside the stale period")
	}
}

func TestFocusFetchLeavesABusyWorkspaceAlone(t *testing.T) {
	_, pump, sh, bare := pushShell(t)
	otherPushes(t, bare, "six.txt")
	ageFetch(t, sh, time.Hour)
	sh.git.opBusy = true
	before := originMain(t, sh)
	focus(sh)
	sh.autoFetchTick() // the timer's path reaches the same guard
	idle(pump)
	if got := originMain(t, sh); got != before {
		t.Error("a fetch started over a running Git operation")
	}
}

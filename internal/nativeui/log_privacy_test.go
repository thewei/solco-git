package nativeui

/**
 * [INPUT]: git_operations.go、window.go OpenRepository/OpenURL、带文件的临时仓库、headless 窗口
 * [OUTPUT]: 日志合同：打开仓库、跑一次 Git 操作、收到一条被拒的 solcogit:// 链接之后，日志里有操作名与标签数，却没有仓库路径、目录名、文件名或链接参数
 * [POS]: README / AGENTS 对日志的承诺（只写操作名、计数、错误类别与耗时）的回归防线；抓取本身也被断言过，空抓取不会让测试假绿
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

// syncBuffer is a log sink the background Git goroutine may write to while the
// test thread reads.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// captureLogs routes the default logger into a buffer for one test, at the
// level the app logs at unless SOLCOGIT_LOG_LEVEL asks for debug (the one
// gitworkbench line that names a repository root is a debug line). The time
// attribute is dropped so a timestamp's digits cannot collide with a name.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return buf
}

func TestLogsNeverNameTheRepository(t *testing.T) {
	root := gitFixture(t, "secretproject")
	w, pump := newTestWindow(t)
	logs := captureLogs(t)

	sh := openAndLoad(t, w, pump, root)
	w.OpenRepository(root, viewFiles, "src/lib/util.go") // a file named in the link
	sh.runGitOp("Fetch", func(ctx context.Context, repo string) (string, error) { return "", nil })
	pump.until(t, "the Git operation to finish", func() bool { return !sh.git.opBusy })
	w.OpenURL("solcogit://open?repo=%2Fsecretfolder%00&file=secretfile.txt") // refused: it carries a NUL

	out := logs.String()
	for _, want := range []string{"git op start", "git op done", "repository shown", "workspaces=1", "ignored unsupported deep link"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log lacks %q, so this test would prove nothing:\n%s", want, out)
		}
	}
	for _, secret := range []string{root, "secretproject", "util.go", "secretfolder", "secretfile"} {
		if strings.Contains(out, secret) {
			t.Errorf("log carries %q:\n%s", secret, out)
		}
	}
}

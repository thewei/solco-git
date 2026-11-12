package nativeui

import (
	"context"
	"os"
	"testing"

	"github.com/thewei/solco-gitkit/engine"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// Git runs this test binary as its editor while the engine rewrites history.
func TestMain(m *testing.M) {
	if engine.RunDaemon() {
		return
	}
	os.Exit(m.Run())
}

// The Git service's reads of the local branches come from the LazyGit engine,
// and the answer is the one the argv path gives.
func TestGitServiceBranchesComeFromTheEngine(t *testing.T) {
	root := gitFixture(t, "engine")
	svc := newGitService()
	if svc.runner.Backend == nil || svc.runner.Backend.Name() != "lazygit" {
		t.Fatalf("the service's runner must carry the LazyGit engine, got %v", svc.runner.Backend)
	}
	got, err := svc.runner.ListBranches(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := (&gitworkbench.Runner{}).ListBranches(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "feature" || got[1].Name != "main" || !got[1].Current {
		t.Fatalf("unexpected branches: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("engine %+v and argv %+v disagree", got[i], want[i])
		}
	}
}

// SOLCOGIT_GIT_BACKEND=argv is the way back: no backend at all.
func TestGitBackendSwitchRestoresTheArgvPath(t *testing.T) {
	t.Setenv("SOLCOGIT_GIT_BACKEND", "argv")
	if svc := newGitService(); svc.runner.Backend != nil {
		t.Fatalf("the switch must leave the runner without a backend, got %v", svc.runner.Backend)
	}
	root := gitFixture(t, "argv")
	branches, err := newGitService().runner.ListBranches(context.Background(), root)
	if err != nil || len(branches) != 2 {
		t.Fatalf("the argv path must still answer: %v %v", branches, err)
	}
}

// A history rewrite on the service's runner goes through LazyGit's rebase with
// this very binary as Git's editor, and leaves a clean repository.
func TestGitServiceRewritesHistoryThroughTheEngine(t *testing.T) {
	root := gitFixture(t, "rewrite")
	gitRun(t, root, "stash", "push", "-u", "-q") // the fixture is dirty on purpose
	head := gitRun(t, root, "rev-parse", "HEAD")
	svc := newGitService()
	err := svc.runner.RewriteHistory(context.Background(), root, gitworkbench.HistoryOp{
		Action: gitworkbench.HistorySquash, Hashes: []string{head}})
	if err != nil {
		t.Fatal(err)
	}
	if got := gitRun(t, root, "log", "--format=%s", "--reverse"); got != "Initial commit\nAdd main" {
		t.Fatalf("history after squash: %q", got)
	}
	if got := gitRun(t, root, "status", "--porcelain"); got != "" {
		t.Fatalf("work tree not clean: %q", got)
	}
}

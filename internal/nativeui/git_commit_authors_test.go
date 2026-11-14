package nativeui

/**
 * [INPUT]: commit_surface.go 的 Co-authors 与 Reset author、git_service_mutation.go 的 submitCommitAmend、真实临时仓库
 * [OUTPUT]: 提交作者选项的界面合同：Co-authors 字段（分号分隔的 Name <email>）写成 Co-authored-by 尾注，提交成功后清空；格式不对时拒绝并留在表单里，不产生提交
 * [POS]: 2026-11-14 LazyGit 对齐批次；内核合同在 solco-gitkit gitworkbench/commit_authors_test.go
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"strings"
	"testing"
)

func commitScratch(t *testing.T, name, coAuthors string) (*Shell, *uiPump, string) {
	t.Helper()
	root := gitFixture(t, name)
	writeFile(t, filepath.Join(root, "scratch.txt"), "x\n")
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.surface.commit.Selected = map[string]bool{"scratch.txt": true}
	sh.openCommitSurface()
	pump.until(t, "the preflight", func() bool { return sh.git.preflight != nil && sh.git.preflightSnap != nil })
	sh.surface.commit.Selected = map[string]bool{"scratch.txt": true}
	sh.surface.commit.CoAuthors = coAuthors
	return sh, pump, root
}

func TestCoAuthorsFieldBecomesTrailersAndClears(t *testing.T) {
	sh, pump, root := commitScratch(t, "coauthors", "Ada Example <ada@example.com>; Bob <bob@example.com>")
	sh.submitCommitAmend("pair work", "", []string{"scratch.txt"}, false)
	pump.until(t, "the commit", func() bool {
		return !sh.git.committing && strings.HasPrefix(gitRun(t, root, "log", "-1", "--format=%s"), "pair work")
	})
	got := strings.TrimSpace(gitRun(t, root, "log", "-1", "--format=%(trailers:key=Co-authored-by,valueonly)"))
	if got != "Ada Example <ada@example.com>\nBob <bob@example.com>" {
		t.Errorf("trailers = %q", got)
	}
	if sh.surface.commit.CoAuthors != "" {
		t.Errorf("the field clears after a commit: %q", sh.surface.commit.CoAuthors)
	}
}

func TestBadCoAuthorStaysInTheFormWithoutACommit(t *testing.T) {
	sh, pump, root := commitScratch(t, "coauthor-bad", "just a name")
	before := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD"))
	sh.submitCommitAmend("never", "", []string{"scratch.txt"}, false)
	pump.until(t, "the refusal", func() bool { return !sh.git.committing && sh.surface.commit.ErrText != "" })
	if !strings.Contains(sh.surface.commit.ErrText, "Name <email>") {
		t.Errorf("the refusal should say what shape is wanted: %q", sh.surface.commit.ErrText)
	}
	if sh.surface.commit.CoAuthors != "just a name" {
		t.Errorf("the field keeps what was typed: %q", sh.surface.commit.CoAuthors)
	}
	if got := strings.TrimSpace(gitRun(t, root, "rev-parse", "HEAD")); got != before {
		t.Error("a refused co-author must not commit")
	}
}

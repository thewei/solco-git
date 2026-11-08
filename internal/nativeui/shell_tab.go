package nativeui

/**
 * [INPUT]: 依赖 shell.go 的 Shell 与 git/files 状态、window.go 的 Window（标签条宿主）
 * [OUTPUT]: 对外提供 Shell.arrive（标签来到最前时对齐 Git 上下文）、pollGit（前台标签的后台刷新节拍）、commitDraftPresent/gitBusy（关闭确认读的两个事实）、revealFile（外部链接在 Files 里定位文件）、selectChangedFile（外部链接在 Changes 里选中改动文件，不跳到 Files；首读前到达的链接由 applyGitSnapshot 在首读落地后滚到该卡）、Close（标签关闭时放掉计时器与预览服务）
 * [POS]: 一个标签作为窗口成员的生命周期接缝；不含任何 Git 行为（那些在复制来的 git_*.go 里）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"path/filepath"
	"strings"
)

// arrive aligns the workspace's Git context when it comes to the front: the root is
// read, the snapshot refreshed when due, and the Auto-fetch timer armed.
func (s *Shell) arrive() {
	s.syncGitContext()
	s.ensureGitSnapshot(false)
	s.ensureAutoFetch()
}

// pollGit is the front workspace's beat: the snapshot cache decides whether a
// background refresh is due, and the Auto-fetch timer is re-armed once a
// repository has been read.
func (s *Shell) pollGit() {
	if s.git == nil || s.git.root == "" {
		return
	}
	s.ensureGitSnapshot(false)
	s.ensureAutoFetch()
}

// commitDraftPresent reports a commit message being written: in the one
// Commit editor, or in the quick bar under the diff.
func (s *Shell) commitDraftPresent() bool {
	if s.surface.current() == WorkspaceSurfaceCommit &&
		(strings.TrimSpace(s.surface.commit.Subject) != "" || strings.TrimSpace(s.surface.commit.Body) != "") {
		return true
	}
	return strings.TrimSpace(s.gitQuickSubject) != "" || strings.TrimSpace(s.gitQuickBody) != ""
}

// gitBusy reports a Git operation still running in this workspace.
func (s *Shell) gitBusy() bool {
	return s.git != nil && (s.git.opBusy || s.git.committing || s.git.branchBusy)
}

// revealFile opens a file in the Files view, expanding its folders in the
// tree. A path outside the repository is ignored.
func (s *Shell) revealFile(file string) {
	abs := file
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.dir, file)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(s.dir, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	s.setView(viewFiles)
	for dir := filepath.Dir(abs); dir != s.dir && strings.HasPrefix(dir, s.dir); dir = filepath.Dir(dir) {
		s.rightPanel.expandedDirs[dir] = true
	}
	s.openFilePreview(abs)
}

// selectChangedFile picks a file on Changes the way a click on its row does:
// selected, its folders open, its card scrolled to the top. A path outside the
// repository, or one the loaded snapshot does not list as changed, selects
// nothing; with no snapshot read yet the path is kept and the first read drops
// it again when it is not changed (GWB-183).
func (s *Shell) selectChangedFile(file string) {
	rel := file
	if filepath.IsAbs(rel) {
		r, err := filepath.Rel(s.dir, rel)
		if err != nil {
			return
		}
		rel = r
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return
	}
	if snap := s.gitSnapshot(); snap != nil {
		if _, ok := snap.ByPath(rel); !ok {
			return
		}
	}
	s.openChanges(rel)
	s.revealDiffFile(rel)
	if s.gitSnapshot() == nil && s.git != nil {
		s.git.pendingReveal = rel // no cards yet: the first read scrolls to it
	}
}

// Close lets go of what the workspace started: the Auto-fetch timer, an in-flight
// root resolution and the static preview server.
func (s *Shell) Close() {
	if s.gitMore.autoFetchTimer != nil {
		s.gitMore.autoFetchTimer.Stop()
		s.gitMore.autoFetchTimer = nil
	}
	if s.git != nil && s.git.rootCancel != nil {
		s.git.rootCancel()
		s.git.rootCancel = nil
	}
	s.closeStaticPreview()
}

// refreshCurrent is ⌘R: re-read what the front view shows, and the remote's
// last-fetch time with it.
func (s *Shell) refreshCurrent() {
	if s.currentView() == viewFiles {
		s.refreshFiles()
	} else {
		s.refreshGitChanges()
	}
	s.checkFetchedAt(true)
}

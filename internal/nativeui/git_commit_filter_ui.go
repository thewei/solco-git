package nativeui

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

/**
 * [INPUT]: gitworkbench.FilterCommits（Git 一侧的路径 / 日期 / 范围查询）、提交列表头（git_review_split.go）、文字搜索（git_list_head.go）
 * [OUTPUT]: 提交列表头里的 Filters 按钮与弹层（Path、Since、Until、Current branch only）、已应用的结果 commits、baseCommits（文字搜索作用的底表）、clear / reload；筛选中的列表只画圆点（行不再相邻），摘要写明 “N of …”，头部有一键清除
 * [POS]: 2026-11-12 决定第 (14) 条的界面；文字搜索仍在已读入的窗口里筛，Filters 是 Git 的查询，两者叠加
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

// commitFilterState is the Filters popover and what Git last answered. The
// edit fields are what the popover shows; applied is what the list was read
// with — typing changes nothing until Apply.
type commitFilterState struct {
	open               bool
	path, since, until string
	currentOnly        bool
	applied            gitworkbench.CommitFilter
	commits            []gitworkbench.CommitInfo
	loading            bool
	err                string
	gen                uint64
}

// commitFilterOn says whether the list is the answer of a Git-side filter.
func (s *Shell) commitFilterOn() bool { return s.gitMore.cfilter.applied.Active() }

// baseCommits is the list the text search works on: the Git-side filter's
// answer when one is applied, else the graph's commits.
func (s *Shell) baseCommits() []gitworkbench.CommitInfo {
	if s.commitFilterOn() {
		return s.gitMore.cfilter.commits
	}
	return s.git.commits
}

// applyCommitFilter reads the edited filter from Git. An empty one is a clear.
func (s *Shell) applyCommitFilter() {
	f := &s.gitMore.cfilter
	next := gitworkbench.CommitFilter{
		Path: strings.TrimSpace(f.path), Since: strings.TrimSpace(f.since), Until: strings.TrimSpace(f.until),
		CurrentBranchOnly: f.currentOnly,
	}
	if !next.Active() {
		s.clearCommitFilter()
		return
	}
	f.applied = next
	s.reloadCommitFilter()
}

// reloadCommitFilter asks Git again with the applied filter (the history moved
// on, or the filter just changed). The old answer stays until the new lands.
func (s *Shell) reloadCommitFilter() {
	if s.git == nil || s.git.root == "" || !s.commitFilterOn() {
		return
	}
	f := &s.gitMore.cfilter
	q, root := f.applied, s.git.root
	q.Hidden, q.Limit = slices.Clone(s.graphHiddenList()), s.graphWindow()
	f.gen++
	gen := f.gen
	f.loading, f.err = true, ""
	runner := s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		commits, err := runner.FilterCommits(ctx, root, q)
		s.applyGuarded(func() uint64 { return s.gitMore.cfilter.gen }, gen, func() {
			if s.git == nil || s.git.root != root {
				return
			}
			f := &s.gitMore.cfilter
			f.loading = false
			if err != nil {
				f.err = gitFailureText(err)
				return
			}
			f.commits = commits
		})
	}()
}

// clearCommitFilter drops the Git-side filter and its answer.
func (s *Shell) clearCommitFilter() {
	f := &s.gitMore.cfilter
	gen := f.gen + 1
	*f = commitFilterState{gen: gen}
}

// commitFilterButton is the Filters button of the Commits head and its popover.
func (s *Shell) commitFilterButton(c *ui.Context) {
	k := gorexColorsOf(c.Theme().Dark)
	f := &s.gitMore.cfilter
	label := "Filter commits by path, date or branch"
	btn := gorexIconButton(c, k, iconFilter, "Commit filters", label, f.open || s.commitFilterOn(), gorexPaneBtn, gorexPaneGlyph)
	if btn.Clicked() {
		f.open = !f.open
		if f.open { // the popover opens showing what is applied
			a := f.applied
			f.path, f.since, f.until, f.currentOnly = a.Path, a.Since, a.Until, a.CurrentBranchOnly
		}
	}
	if f.open {
		ui.Popover(c, btn, &f.open, func() { s.commitFilterPanel(c) })
	}
}

func (s *Shell) commitFilterPanel(c *ui.Context) {
	t := c.Theme()
	sp := Spacing()
	f := &s.gitMore.cfilter
	ui.Column(c).Width(320).Padding(sp.M).Gap(sp.S).Children(func() {
		submit := false
		captionedField(c, "Path", "A file or folder, relative to the repository", func() {
			submit = textField(c, &f.path).Label("Filter path").Placeholder("internal/app").FillWidth().Submitted() || submit
		})
		ui.Row(c).FillWidth().Gap(sp.S).Children(func() {
			ui.Column(c).Grow(1).Basis(0).Children(func() {
				captionedField(c, "Since", "", func() {
					submit = textField(c, &f.since).Label("Filter since").Placeholder("YYYY-MM-DD").FillWidth().Submitted() || submit
				})
			})
			ui.Column(c).Grow(1).Basis(0).Children(func() {
				captionedField(c, "Until", "", func() {
					submit = textField(c, &f.until).Label("Filter until").Placeholder("YYYY-MM-DD").FillWidth().Submitted() || submit
				})
			})
		})
		ui.Checkbox(c, &f.currentOnly, "Current branch only")
		if f.err != "" {
			ui.Text(c, f.err).FontSize(Typography().Caption).TextColor(t.Danger)
		}
		ui.Row(c).FillWidth().Gap(sp.S).Justify(ui.End).Children(func() {
			if compactButton(c, "Clear").Disabled(!s.commitFilterOn() && f.path == "" && f.since == "" && f.until == "" && !f.currentOnly).Clicked() {
				s.clearCommitFilter()
				f.open = false
			}
			if compactPrimaryButton(c, "Apply").Clicked() {
				submit = true
			}
		})
		if submit {
			s.applyCommitFilter()
			f.open = false
		}
	})
}

// commitFilterSummary is the text that says what the list is filtered by.
func (s *Shell) commitFilterSummary() string {
	a := s.gitMore.cfilter.applied
	var parts []string
	if a.Path != "" {
		parts = append(parts, a.Path)
	}
	switch {
	case a.Since != "" && a.Until != "":
		parts = append(parts, a.Since+" – "+a.Until)
	case a.Since != "":
		parts = append(parts, "since "+a.Since)
	case a.Until != "":
		parts = append(parts, "until "+a.Until)
	}
	if a.CurrentBranchOnly {
		parts = append(parts, "current branch")
	}
	return strings.Join(parts, " · ")
}

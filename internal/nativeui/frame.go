package nativeui

/**
 * [INPUT]: 依赖 shell.go 的 Shell（一个仓库标签）、git_* 的快照/分支/同步动作、files 的树与搜索、window.go 的 Window（设置、标签）
 * [OUTPUT]: 对外提供一个标签的窗口外壳：viewTabs（Changes｜Commits｜Files 三行，直接铺在侧栏材质上：一行一个图形加名称，选中的是整行铺满的侧栏选中色，其余悬停时才有悬停色；Changes 的计数贴行尾，行下一条细线；仍是名为 View 的单选组，提示里有 ⌘1/⌘2/⌘3）、repoHead（仓库名 + 搜索 + 刷新）、sidebarColumn（侧栏正文）、sidebarFoot（工作区芯片 + Settings，同 Solco 页脚）、titleTools（标题带右侧：文件栏开关/Split·Unified、Fetch/Pull/Push、Actions）、branchChip + branchSwitcherPanel（分支下拉）、stage（中央：Changes/Commits 评审面或 Files 页）、fetchTip/fetchedAgo/checkFetchedAt（Fetch 钮提示里的"上次抓取"，原来页脚那句 Fetched … ago 搬到这里）
 * [POS]: Solco Git 的版面落点：把复制来的 Git 评审面与 Files 页装进设计稿的三栏（侧栏｜标题带｜舞台）；侧栏顶是三个视图，不再有 Solco 那样的 Terminal/Sessions/Hosts 标签
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// frameSync is the front workspace's per-frame prelude: the repository root is read
// once, and the footer's "Fetched …" is re-checked now and then.
func (s *Shell) frameSync() {
	s.syncGitContext()
	s.viewFrame++
	s.checkFetchedAt(false)
}

// ---- the sidebar -----------------------------------------------------------

// sidebarViews are the sidebar's three destinations, in tab order: the name is
// the tab's accessible name and tooltip, the glyph its face, accel the menu
// shortcut its tooltip spells.
var sidebarViews = [...]struct {
	kind  viewKind
	label string
	glyph *ui.SVG
	accel string
}{
	{viewChanges, "Changes", iconFileDiff, accelChanges},
	{viewCommits, "Commits", iconCommit, accelCommits},
	{viewFiles, "Files", iconFolder, accelFiles},
}

// viewTabs is the sidebar's first block: the three views as three rows, one
// glyph and one name each, straight on the sidebar's material (2026-11-12, the
// TanGit review; it was a tab strip). The chosen row is the sidebar's selection
// wash across the full row, the others rest on the material and take the hover
// wash. The Changes row carries the changed-file count at its right edge; a
// hairline closes the block.
func (s *Shell) viewTabs(c *ui.Context) {
	chosen := 0
	for i, v := range sidebarViews {
		if v.kind == s.currentView() {
			chosen = i
		}
	}
	changes := 0
	if snap := s.gitSnapshot(); snap != nil {
		changes = len(snap.Files)
	}
	ui.Column(c).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Padding(2, sidebarGutter, 6).Children(func() {
			before := chosen
			materialNavGroup(c, "View", func() {
				for i, v := range sidebarViews {
					count := 0
					if v.kind == viewChanges {
						count = changes
					}
					row, ink := materialNavRow(c, &chosen, i, v.label, s.sidebarRowHeight())
					tip := v.label
					if count > 0 {
						tip += " · " + pluralFiles(count)
					}
					tipped(c, row, tip, tipKeys(acceleratorKeycaps(v.accel)))
					row.Children(func() {
						ui.Icon(c, v.glyph).Size(gorexIconGlyph, gorexIconGlyph).TextColor(ink).Shrink(0)
						ui.Text(c, v.label).FontSize(Typography().Body).FontWeight(WeightMedium).TextColor(ink).
							SingleLine().Grow(1).MinWidth(0)
						if count > 0 {
							ui.Text(c, itoa(count)).FontSize(Typography().Caption).FontWeight(WeightSemibold).
								TextColor(ink).SingleLine().Shrink(0)
						}
					})
				}
			}).FillWidth()
			if chosen != before {
				s.showView(sidebarViews[chosen].kind)
			}
		})
		ui.Divider(c)
	})
}

// repoHead is the sidebar's head: the repository's name (its folder in the
// tooltip), the search that filters what the sidebar lists, and Refresh.
func (s *Shell) repoHead(c *ui.Context) {
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	files := s.currentView() == viewFiles
	search := &headerSearch{
		Query: &s.gitRefsFilter, Open: &s.gitRefsSearchOpen, Label: "Filter Git references", Button: "Search references",
		Changed: s.gitRefsFilterChanged,
	}
	refreshLabel, refresh := "Refresh repository", s.refreshGitChanges
	if files {
		search = &headerSearch{
			Query: &s.filesSearch.query, Open: &s.filesSearch.open, Label: "Search files", Button: "Search files",
			Changed: s.filesSearchChanged, Submit: s.filesSearchOpenFirst,
		}
		refreshLabel, refresh = "Refresh files", s.refreshFiles
	}
	ui.Column(c).FillWidth().Children(func() {
		ui.Row(c).FillWidth().Padding(6, sidebarGutter, 4).Gap(2).AlignItems(ui.Center).Children(func() {
			name := s.repoName()
			title := ui.Text(c, name).FontSize(Typography().Body).FontWeight(WeightSemibold).SingleLine().
				Ellipsis("…").Shrink(1).MinWidth(0).Grow(1)
			tipped(c, title, tildePath(s.dir))
			search.button(c, k)
			if gorexIconButton(c, k, iconRefresh, refreshLabel, refreshLabel, false, gorexPaneBtn, gorexPaneGlyph).Clicked() {
				refresh()
			}
		})
		if *search.Open {
			ui.Row(c).FillWidth().Padding(0, sidebarGutter, 6).Children(func() { search.field(c) })
		}
	})
}

// repoName is the folder name the workspace chip and the sidebar head show.
func (s *Shell) repoName() string {
	if name := s.gitRepoName(); name != "" {
		return name
	}
	return pathBase(s.dir)
}

// sidebarColumn is the sidebar below the traffic-light band: the three views,
// the repository head, the references tree (Changes, Commits) or the file
// tree (Files), and the foot.
func (s *Shell) sidebarColumn(c *ui.Context) {
	ui.Column(c).Width(s.sidebarW).Grow(1).MinHeight(0).Shrink(0).Children(func() {
		s.viewTabs(c)
		s.repoHead(c)
		ui.Divider(c)
		// Each body keeps its own identity: the refs tree and the file tree
		// are different lists, and sharing a slot let rows of the one linger
		// under the other.
		if s.currentView() == viewFiles {
			ui.Column(c.Key("sidebar-files")).Grow(1).MinHeight(0).Children(func() {
				s.syncRightPanelRoot()
				s.filesToolView(c)
			})
		} else {
			ui.Column(c.Key("sidebar-refs")).Grow(1).MinHeight(0).Children(func() { s.gitRepositorySidebar(c) })
		}
		s.sidebarFoot(c)
	})
}

// sidebarFoot is the sidebar's bottom bar, laid out as Solco's is: the
// workspace chip filling the row (its pull-down opens upward) and the Settings
// button.
func (s *Shell) sidebarFoot(c *ui.Context) {
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	ui.Row(c).FillWidth().Height(42).Shrink(0).Padding(0, sidebarGutter).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(s.tokens(t.Dark).BorderSubtle).Children(func() {
		if s.w != nil {
			s.w.workspaceSwitcher(c, k)
		} else {
			ui.Spacer(c)
		}
		settings := gorexIconButton(c, k, iconSettings, "Settings", "", false, gorexIconBtn, gorexIconGlyph)
		tipped(c, settings, "Settings", tipKeys(acceleratorKeycaps(accelSettings)))
		if settings.Clicked() && s.w != nil {
			s.w.openSettings()
		}
	})
}

// fetchTip is the Fetch button's tooltip: what it does, and — where it can
// say — when the remotes were last fetched (the line the sidebar's foot wore
// before the workspace chip took its place).
func (s *Shell) fetchTip() string {
	tip := "Fetch from the remote"
	if label, can := s.fetchedLabel(); can {
		tip += " — " + label
	}
	return tip
}

// fetchedLabel is the words for when the remotes were last fetched, and
// whether there is anything to fetch (a repository with a remote).
func (s *Shell) fetchedLabel() (text string, canFetch bool) {
	g := s.git
	if g == nil || g.root == "" {
		return "No repository", false
	}
	if len(g.remotes) == 0 {
		return "No remote", false
	}
	if !g.fetchedOK {
		return "Not fetched yet", true
	}
	return "Fetched " + fetchedAgo(timeNow().Sub(g.fetchedAt)), true
}

// fetchedAgo is how long ago a fetch was, as the footer says it.
func fetchedAgo(d time.Duration) string {
	unit := func(n int, one, many string) string {
		if n == 1 {
			return fmt.Sprintf("1 %s ago", one)
		}
		return fmt.Sprintf("%d %s ago", n, many)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return unit(int(d/time.Minute), "min", "min")
	case d < 24*time.Hour:
		return unit(int(d/time.Hour), "hr", "hr")
	case d < 30*24*time.Hour:
		return unit(int(d/(24*time.Hour)), "day", "days")
	case d < 365*24*time.Hour:
		return unit(int(d/(30*24*time.Hour)), "month", "months")
	}
	return unit(int(d/(365*24*time.Hour)), "year", "years")
}

// checkFetchedAt re-reads the footer's fetch time off the UI lane: at most
// every 15 seconds on its own, at once (force) after a Git operation.
func (s *Shell) checkFetchedAt(force bool) {
	g := s.git
	if g == nil || g.root == "" || g.runner == nil {
		return
	}
	now := timeNow()
	if !force && now.Sub(g.fetchedChecked) < 15*time.Second {
		return
	}
	g.fetchedChecked = now
	root := g.root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout)
		defer cancel()
		at, ok := g.runner.FetchedAt(ctx, root)
		s.applyOnUI(func() {
			if s.git != nil && s.git.root == root {
				s.git.fetchedAt, s.git.fetchedOK = at, ok
			}
		})
	}()
}

// ---- the title band's tools ------------------------------------------------

// branchChip is the band's branch pull-down: the branch, how many commits it
// is ahead of its upstream, and the chevron. It opens branchSwitcherPanel.
func (s *Shell) branchChip(c *ui.Context) ui.Element {
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	name := s.currentBranchName()
	tip := "Switch branch"
	if name == "" {
		snap := s.gitSnapshot()
		switch {
		case snap == nil:
			// The first read has not landed: say nothing about the head rather
			// than claim a detached one for the beat before Git answers.
			name = "…"
		case snap.Head != "":
			name = "HEAD " + shortHash(snap.Head)
		default:
			name = "Detached HEAD"
		}
	}
	ahead := 0
	if up := s.git.upstream; up.OK {
		ahead = up.Ahead
	}
	chip := tipped(c, ui.ButtonBase(c.Key("band-branch")).Height(Controls().RegularHeight).Padding(0, 10).Gap(7).
		Radius(Radius().Row).AlignItems(ui.Center).Shrink(1).MinWidth(0).Label("Switch branch"), tip)
	gorexToggleFace(chip, k, s.branchMenuOpen)
	chip.Children(func() {
		ui.Icon(c, iconBranch).Size(Controls().IconRegular, Controls().IconRegular).TextColor(t.TextMuted).Shrink(0).PassThrough()
		ui.Text(c, name).FontSize(Typography().Body).FontWeight(WeightSemibold).TextColor(t.Text).
			SingleLine().Ellipsis("…").Shrink(1).MinWidth(0).PassThrough()
		if ahead > 0 {
			ui.Text(c, "↑"+itoa(ahead)).FontSize(Typography().BodySmall).FontWeight(WeightSemibold).
				TextColor(t.Accent).SingleLine().Shrink(0).PassThrough()
		}
		ui.Icon(c, iconChevronDown).Size(Controls().IconSmall, Controls().IconSmall).TextColor(t.TextMuted).Shrink(0).PassThrough()
	})
	wasOpen := s.branchMenuOpen
	toggleMenu(&s.branchMenuOpen, chip)
	if s.branchMenuOpen && !wasOpen {
		s.branchFilter = ""
	}
	if s.branchMenuOpen {
		ui.Popover(c, chip, &s.branchMenuOpen, func() { s.branchSwitcherPanel(c) })
	}
	return chip
}

// branchSwitcherPanel lists the local branches, current first, for a
// one-click switch. Merge and delete stay on each row's context menu; a
// footer row starts a new branch.
func (s *Shell) branchSwitcherPanel(c *ui.Context) {
	t := c.Theme()
	sp := Spacing()
	ui.Column(c).Width(320).Padding(sp.S).Gap(sp.XS).Children(func() {
		matches := matchingLocalBranches(s.git.branches, s.branchFilter)
		if switcherSearch(c, &s.branchFilter, "Search branches") {
			for _, b := range matches {
				if !b.Current {
					s.branchMenuOpen = false
					s.confirmSwitchBranch(b.Name)
					return
				}
			}
		}
		ui.Scroll(c).FillWidth().MaxHeight(260).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				if s.git.branchesLoading && len(matches) == 0 {
					ui.Row(c).FillWidth().Padding(sp.XS, sp.S).Gap(sp.XS).AlignItems(ui.Center).Children(func() {
						ui.Spinner(c).Size(12, 12)
						ui.Text(c, "Loading branches…").FontSize(Typography().Caption).TextColor(t.TextMuted)
					})
					return
				}
				if len(matches) == 0 {
					label := "No local branches"
					if strings.TrimSpace(s.branchFilter) != "" {
						label = "No matching branches"
					}
					ui.Text(c, label).FontSize(Typography().Caption).TextColor(t.TextMuted).Padding(sp.S)
				}
				for _, branch := range matches {
					b := branch
					row := crumbItemRow(c, iconBranch, b.Name, b.Current)
					if row.Clicked() && !b.Current {
						s.branchMenuOpen = false
						s.confirmSwitchBranch(b.Name)
					}
					if !b.Current {
						row.ContextMenu(func(m *ui.Menu) {
							if m.Item("Merge into Current…").Chosen() {
								s.branchMenuOpen = false
								s.openMergeDialog(b.Name)
							}
							if m.Item("Delete Branch…").Chosen() {
								s.branchMenuOpen = false
								s.deleteBranch(b.Name)
							}
						})
					}
				}
			})
		})
		ui.Divider(c)
		create := crumbItemRow(c, iconPlus, "New Branch…", false)
		if create.Clicked() {
			s.branchMenuOpen = false
			s.openNewBranchDialog()
		}
	})
}

// syncButtons are the band's Fetch, Pull and Push — labeled, on every view.
// Push carries the number of commits waiting; a Git operation in flight shows
// as a spinner in their place; a branch that diverged from its upstream gets
// a Push that says why it cannot push.
func (s *Shell) syncButtons(c *ui.Context) {
	if s.git == nil || s.git.root == "" || s.currentBranchName() == "" {
		return
	}
	t := c.Theme()
	k := gorexColorsOf(t.Dark)
	if s.git.opBusy {
		tipped(c, ui.Spinner(c).Size(14, 14).Label("Git operation in progress…"), "Git operation in progress…")
		return
	}
	paused := s.git.sequencerChecked && s.git.sequencer.Active()
	act := func(b ui.Element, run func(), off bool) {
		if off {
			b.Disabled(true)
		}
		if b.Clicked() && !off {
			run()
		}
	}
	act(bandAction(c, k, iconFetch, "Fetch", s.fetchTip(), 0, false), s.fetchRepo, paused)
	act(bandAction(c, k, iconPull, "Pull", "Pull (fast-forward only)", s.git.upstream.Behind, false), s.pullRepo, paused)
	up := s.git.upstream
	switch {
	case up.Diverged():
		act(bandAction(c, k, iconPush, "Push", "Can't push: "+up.Remote+"/"+up.Branch+
			" has commits you don't — merge or rebase first", up.Ahead, false), s.pushRepo, true)
	default:
		act(bandAction(c, k, iconPush, "Push", "Push", up.Ahead, up.Ahead > 0), s.pushRepo, paused)
	}
}

// titleTools is the right of the title band for a workspace: the Changes
// view's file-list toggle and Split/Unified switch, Fetch/Pull/Push, and the
// Actions menu (Git actions on Changes and Commits, path actions on Files).
func (s *Shell) titleTools(c *ui.Context) {
	sp := Spacing()
	ui.Row(c).Gap(sp.XS).AlignItems(ui.Center).Shrink(0).Children(func() {
		if s.gitHeaderToolsShown() {
			s.gitFilePaneToggle(c)
			s.gdLayoutControl(c)
		}
		s.syncButtons(c)
		s.terminalToggle(c)
		switch s.currentView() {
		case viewFiles:
			headerActions(c, s.filesActionItems)
		default:
			if s.gitSnapshot() != nil {
				headerActions(c, s.gitActionItems)
			}
		}
	})
}

// filesActionItems is the Files view's Actions menu: the open file's path
// actions, and the tree-wide ones.
func (s *Shell) filesActionItems(m *ui.Menu) {
	path := s.rightPanel.selectedFilePath
	if path != "" {
		s.filesPathItems(m, path, true)
		m.Separator()
	}
	if m.Item("New File…").Chosen() {
		s.filesNewFile(s.rightPanel.currentRoot)
	}
	if m.Item("New Folder…").Chosen() {
		s.filesNewFolder(s.rightPanel.currentRoot)
	}
	m.Separator()
	if m.Item("Show Ignored Files").Checked(s.rightPanel.showIgnored).Chosen() {
		s.rightPanel.showIgnored = !s.rightPanel.showIgnored
		s.filesSearchChanged()
	}
	if m.Item("Refresh Files").Chosen() {
		s.refreshFiles()
	}
}

// ---- the stage -------------------------------------------------------------

// stage is the content column under the title band: the reconnect-style error
// banner, then the Changes/Commits review surface or the Files page.
func (s *Shell) stage(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).MinWidth(0).Children(func() {
		if s.errText != "" {
			ui.Row(c).Padding(7, 12).Background(t.Danger.Alpha(0.12)).BorderWidth(0, 0, 1, 0).BorderColor(t.Danger.Alpha(0.35)).Children(func() {
				ui.Text(c, s.errText).TextColor(t.Danger).FontSize(Typography().BodySmall).Grow(1).SingleLine()
			})
		}
		if s.currentView() == viewFiles {
			s.filesPage(c)
		} else {
			gorexContentCard(c, gorexColorsOf(t.Dark), func() { s.gitSplitReviewSurface(c) })
		}
		_, windowH := c.Size()
		s.terminalDrawer(c, windowH-termPageChrome)
	})
}

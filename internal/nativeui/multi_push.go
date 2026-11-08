package nativeui

/**
 * [INPUT]: 依赖 Shell 的仓库状态（git.remotes/upstream/runner）、gitworkbench 的 ReadPushOffer/PushMany、runGitNetworkOp 的单飞网络操作、ds_components 的控件
 * [OUTPUT]: 对外提供 multiPushState、openMultiPush（打开对话框）、multiPushDialog（远端选择、分支清单与标签清单、Set upstream、足迹说明与 Push N refs）、pushChosen（一次 git push 推已勾选的引用）
 * [POS]: Fork 的 Push 对话框：一次把多个分支与标签推到同一个远端；只列"有东西可推"的分支与本地标签，分叉的分支标为受阻而不是强推；工具栏的 Push 仍是推当前分支的一键操作
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// multiPushState is the Push dialog.
type multiPushState struct {
	open    bool
	root    string
	remotes []string
	remote  string
	loading bool
	err     string

	branches, tags []gitworkbench.PushRef
	picked         map[string]bool // "b:<branch>" / "t:<tag>"
	setUpstream    bool
	gen            int
}

func pickKey(ref gitworkbench.PushRef) string {
	if ref.Kind == gitworkbench.PushTag {
		return "t:" + ref.Name
	}
	return "b:" + ref.Name
}

// openMultiPush opens the Push dialog on the current repository, at the
// remote the current branch tracks (else origin, else the first).
func (s *Shell) openMultiPush() {
	if s.git == nil || s.git.root == "" {
		return
	}
	if s.git.opBusy {
		s.pendingToast = "Another Git operation is still running."
		return
	}
	var remotes []string
	for _, r := range s.git.remotes {
		remotes = append(remotes, r.Name)
	}
	if len(remotes) == 0 {
		s.pendingToast = "This repository has no remote to push to."
		return
	}
	remote := remotes[0]
	for _, name := range remotes {
		if name == "origin" && remote != s.git.upstream.Remote {
			remote = name
		}
	}
	if s.git.upstream.OK && s.git.upstream.Remote != "" {
		remote = s.git.upstream.Remote
	}
	s.gitMore.push = multiPushState{open: true, root: s.git.root, remotes: remotes, remote: remote, setUpstream: true}
	s.loadMultiPush()
}

// loadMultiPush reads what the chosen remote could take, in the background.
func (s *Shell) loadMultiPush() {
	d := &s.gitMore.push
	d.gen++
	gen, remote, root := d.gen, d.remote, d.root
	d.loading, d.err = true, ""
	d.branches, d.tags, d.picked = nil, nil, map[string]bool{}
	runner := s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*2)
		defer cancel()
		offer, err := runner.ReadPushOffer(ctx, root, remote)
		s.applyOnUI(func() {
			if gen != d.gen || !d.open {
				return
			}
			d.loading = false
			if err != nil {
				d.err = gitFailureText(err)
				return
			}
			d.branches, d.tags = offer.Branches, offer.Tags
			// The branch you are on is what a Push usually means.
			for _, b := range d.branches {
				if b.Current && b.Blocked == "" {
					d.picked[pickKey(b)] = true
				}
			}
		})
	}()
}

// chosenRefs is what the dialog would push, branches first.
func (d *multiPushState) chosenRefs() (refs []gitworkbench.PushRef, branches, tags int) {
	for _, b := range d.branches {
		if d.picked[pickKey(b)] && b.Blocked == "" {
			refs = append(refs, b)
			branches++
		}
	}
	for _, tg := range d.tags {
		if d.picked[pickKey(tg)] {
			refs = append(refs, tg)
			tags++
		}
	}
	return refs, branches, tags
}

func plural(n int, one, many string) string {
	if n == 1 {
		return itoa(n) + " " + one
	}
	return itoa(n) + " " + many
}

func (s *Shell) multiPushDialog(c *ui.Context) {
	d := &s.gitMore.push
	if !d.open {
		return
	}
	t := c.Theme()
	typ := Typography()
	ui.Modal(c, &d.open, func() {
		title := "Push"
		if d.remote != "" {
			title = "Push to " + d.remote
		}
		modalTitle(c, title)
		ui.Column(c).Width(460).Gap(Spacing().M).Children(func() {
			if len(d.remotes) > 1 {
				captionedField(c, "Remote", "", func() {
					remote := d.remote
					if selectField(c, &remote, d.remotes).Label("Remote").Changed() && remote != d.remote {
						d.remote = remote
						s.loadMultiPush()
					}
				})
			}
			switch {
			case d.loading:
				ui.Row(c).FillWidth().Height(120).Justify(ui.Center).AlignItems(ui.Center).Children(func() { ui.Spinner(c).Size(16, 16) })
			case d.err != "":
				emptyState(c, "Can't list what to push", d.err)
			default:
				s.multiPushLists(c)
			}
			refs, nb, nt := d.chosenRefs()
			ui.Row(c).Gap(Spacing().S).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c.Key("push-upstream"), &d.setUpstream, "")
				ui.Text(c, "Set upstream for branches that have none").FontSize(typ.BodySmall).TextColor(t.TextMuted)
			})
			ui.Row(c).FillWidth().Gap(Spacing().M).AlignItems(ui.Center).Children(func() {
				ui.Text(c, plural(nb, "branch", "branches")+" · "+plural(nt, "tag", "tags")+" · no force push").
					FontSize(typ.Caption).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine()
				if regularButton(c, "Cancel").Clicked() {
					d.open = false
				}
				label := "Push"
				if n := nb + nt; n > 0 {
					label = "Push " + plural(n, "ref", "refs")
				}
				if regularPrimaryButton(c, label).Disabled(len(refs) == 0 || s.git == nil || s.git.opBusy).Clicked() {
					s.pushChosen()
				}
			})
		})
	})
}

// multiPushLists is the dialog's two lists: the branches with something to
// send and the local tags.
func (s *Shell) multiPushLists(c *ui.Context) {
	d := &s.gitMore.push
	t := c.Theme()
	typ := Typography()
	if len(d.branches) == 0 && len(d.tags) == 0 {
		quietNote(c, "Everything is up to date: no branch has commits to send, and there are no tags.")
		return
	}
	rows := len(d.branches) + len(d.tags)
	height := min(float32(rows)*28+72, 320)
	ui.Scroll(c).FillWidth().Height(height).Children(func() {
		ui.Column(c).FillWidth().Gap(2).Children(func() {
			s.multiPushSection(c, "Branches", d.branches, "Nothing to send: every branch is up to date with "+d.remote+".", func(ref gitworkbench.PushRef) {
				meta := ""
				switch {
				case ref.Blocked != "":
					meta = ref.Blocked
				case ref.New:
					meta = "new on " + d.remote
				default:
					meta = plural(ref.Ahead, "commit", "commits") + " to send"
				}
				ink := t.TextMuted
				if ref.Blocked != "" {
					ink = gdPaletteFor(t).delText
				}
				ui.Text(c, meta).FontSize(typ.Caption).TextColor(ink).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			})
			s.multiPushSection(c, "Tags", d.tags, "No local tags.", nil)
		})
	})
}

// multiPushSection is one titled list with its Select all / None action.
func (s *Shell) multiPushSection(c *ui.Context, title string, refs []gitworkbench.PushRef, empty string, meta func(gitworkbench.PushRef)) {
	d := &s.gitMore.push
	t := c.Theme()
	typ := Typography()
	all := len(refs) > 0
	for _, ref := range refs {
		if ref.Blocked == "" && !d.picked[pickKey(ref)] {
			all = false
		}
	}
	ui.Row(c).FillWidth().Gap(Spacing().S).AlignItems(ui.Center).Padding(6, 0, 2).Children(func() {
		ui.Text(c, title).FontSize(typ.BodySmall).FontWeight(WeightSemibold).Grow(1)
		if len(refs) == 0 {
			return
		}
		label := "Select all"
		if all {
			label = "None"
		}
		if sectionTextAction(c, label, "").Clicked() {
			for _, ref := range refs {
				if ref.Blocked == "" {
					d.picked[pickKey(ref)] = !all
				}
			}
		}
	})
	if len(refs) == 0 {
		ui.Text(c, empty).FontSize(typ.Caption).TextColor(t.TextMuted).Padding(2, 0)
		return
	}
	for i := range refs {
		ref := refs[i]
		key := pickKey(ref)
		ui.Row(c.Key("push-" + key)).FillWidth().Height(28).Gap(Spacing().S).AlignItems(ui.Center).Children(func() {
			picked := d.picked[key]
			box := ui.Checkbox(c.Key("push-box-"+key), &picked, "").Label(ref.Name)
			if ref.Blocked != "" {
				box.Disabled(true)
			}
			d.picked[key] = picked && ref.Blocked == ""
			name := ui.Text(c, ref.Name).Font(gdCodeFont()).FontSize(typ.BodySmall).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
			if ref.Blocked != "" {
				name.TextColor(t.TextMuted)
			}
			if ref.Current {
				statusPill(c, "current", ToneNeutral)
			}
			ui.Spacer(c)
			if meta != nil {
				meta(ref)
			}
		})
	}
}

// pushChosen sends the chosen refs in one git push, for the repository and
// remote the dialog was opened on.
func (s *Shell) pushChosen() {
	d := &s.gitMore.push
	refs, nb, nt := d.chosenRefs()
	if len(refs) == 0 || s.git == nil || s.git.root != d.root {
		s.pendingToast = "Repository changed. Reopen the dialog to continue."
		d.open = false
		return
	}
	remote, setUpstream := d.remote, d.setUpstream
	d.open = false
	s.runGitNetworkOp("Push", func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.PushMany(ctx, root, remote, refs, setUpstream); err != nil {
			return "", err
		}
		parts := []string{}
		if nb > 0 {
			parts = append(parts, plural(nb, "branch", "branches"))
		}
		if nt > 0 {
			parts = append(parts, plural(nt, "tag", "tags"))
		}
		return "Pushed " + strings.Join(parts, " and ") + " to " + remote, nil
	})
}

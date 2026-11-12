package nativeui

/**
 * [INPUT]: shell.go（git 状态、applyOnUI）、git_operations.go（runGitOp）、gitworkbench.Runner.ReadConflict / ResolveWith / ResolveConflict、MyGo editor（Result 栏）
 * [OUTPUT]: openConflictEditor（Resolve Conflicts… 的整页弹层）、conflictSheet：每个冲突块一张卡（Current | Incoming 两栏，Base 按需展开，Current / Incoming / Both / Base 四个选择）、下方可手改的 Result、Apply & Stage；不能逐行合并的文件（一方删除、二进制、过大、链接/子模块）只给原因与整文件选择
 * [POS]: 2026-11-12 决定第 (3) 条「冲突三栏编辑」的界面一侧，形态是 VS Code 式（上两栏 + 下 Result，Base 收起）：块级对齐的卡片取代两个整文件编辑器，上下文只留块前后两行；选择块会重写 Result，Result 一旦被手改，块按钮停用并出现 Reset Result，手改不会被悄悄覆盖；Apply 前内核再校验仍在冲突中且结果不含标记，并留恢复点（Recovery 可重开冲突）
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

const (
	conflictContextLines = 2  // unchanged lines kept on each side of a hunk
	conflictShownLines   = 40 // lines of one side a hunk card shows
)

// conflictRow is one row of the hunk list: lines both sides share, a gap, or
// a hunk (by its number among the hunks).
type conflictRow struct {
	lines string // shared text, or "" for a hunk / gap
	gap   int    // unchanged lines hidden
	hunk  int    // 1-based; 0 = not a hunk
}

type conflictSheetState struct {
	open     bool
	loading  bool
	err      string
	path     string
	sides    gitworkbench.ConflictSides
	rows     []conflictRow
	hunks    []*gitworkbench.ConflictHunk
	choices  []gitworkbench.HunkChoice
	derived  string // the Result the choices make
	result   *editor.State
	showBase bool
	list     ui.ListState
	gen      int
}

func (s *Shell) openConflictEditor(path string) {
	if s.git == nil || s.git.root == "" || s.git.opBusy {
		return
	}
	cs := &s.gitMore.conflict
	cs.gen++
	gen := cs.gen
	*cs = conflictSheetState{open: true, loading: true, path: path, gen: gen}
	root, runner := s.git.root, s.git.runner
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gitworkbench.ReadTimeout*3)
		defer cancel()
		sides, err := runner.ReadConflict(ctx, root, path)
		s.applyOnUI(func() {
			cs := &s.gitMore.conflict
			if cs.gen != gen || s.git == nil || s.git.root != root {
				return
			}
			cs.loading = false
			if err != nil {
				cs.err = gitFailureText(err)
				return
			}
			cs.load(sides)
		})
	}()
}

// load builds the rows and the Result from what the kernel read.
func (cs *conflictSheetState) load(sides gitworkbench.ConflictSides) {
	cs.sides = sides
	if !sides.Editable {
		return
	}
	for _, seg := range sides.Segments {
		if seg.Hunk != nil {
			cs.hunks = append(cs.hunks, seg.Hunk)
		}
	}
	cs.choices = make([]gitworkbench.HunkChoice, len(cs.hunks))
	cs.rows = conflictRows(sides.Segments)
	cs.derived = gitworkbench.BuildConflictResult(sides.Segments, cs.choices)
	buf := editor.NewBuffer(cs.derived)
	buf.SetLanguage(editor.LanguageFor(sides.Path))
	cs.result = &editor.State{Buffer: buf, Label: "Result", SoftWrap: buf.Language() == nil}
}

// conflictRows lays the segments out as rows: shared text trimmed to a couple
// of lines beside each hunk, the rest a gap.
func conflictRows(segs []gitworkbench.ConflictSegment) []conflictRow {
	var rows []conflictRow
	hunk := 0
	for i, seg := range segs {
		if seg.Hunk != nil {
			hunk++
			rows = append(rows, conflictRow{hunk: hunk})
			continue
		}
		lines := strings.SplitAfter(seg.Text, "\n")
		if n := len(lines); n > 0 && lines[n-1] == "" {
			lines = lines[:n-1]
		}
		head, tail := conflictContextLines, conflictContextLines
		if i == 0 {
			head = 0 // nothing precedes the first segment
		}
		if i == len(segs)-1 {
			tail = 0
		}
		if len(lines) <= head+tail {
			rows = append(rows, conflictRow{lines: strings.Join(lines, "")})
			continue
		}
		if head > 0 {
			rows = append(rows, conflictRow{lines: strings.Join(lines[:head], "")})
		}
		rows = append(rows, conflictRow{gap: len(lines) - head - tail})
		if tail > 0 {
			rows = append(rows, conflictRow{lines: strings.Join(lines[len(lines)-tail:], "")})
		}
	}
	return rows
}

func (cs *conflictSheetState) edited() bool {
	return cs.result != nil && cs.result.Buffer.String() != cs.derived
}

func (cs *conflictSheetState) undecided() int {
	n := 0
	for _, ch := range cs.choices {
		if ch == gitworkbench.ChoiceNone {
			n++
		}
	}
	return n
}

// choose settles one hunk and rewrites the Result from the choices.
func (cs *conflictSheetState) choose(hunk int, ch gitworkbench.HunkChoice) {
	if cs.edited() || hunk < 1 || hunk > len(cs.choices) {
		return
	}
	cs.choices[hunk-1] = ch
	cs.derived = gitworkbench.BuildConflictResult(cs.sides.Segments, cs.choices)
	cs.result.Buffer.Set(cs.derived)
}

func (cs *conflictSheetState) resetResult() {
	cs.derived = gitworkbench.BuildConflictResult(cs.sides.Segments, cs.choices)
	cs.result.Buffer.Set(cs.derived)
}

func (s *Shell) conflictSheet(c *ui.Context) {
	cs := &s.gitMore.conflict
	if !cs.open {
		return
	}
	t := c.Theme()
	ui.Modal(c, &cs.open, func() {
		ui.Column(c).Width(1040).MaxWidth(1240).Height(700).MinHeight(320).MinWidth(0).Gap(Spacing().M).Children(func() {
			modalTitle(c, "Resolve Conflicts — "+cs.path)
			switch {
			case cs.loading:
				busyNote(c, "Reading the three versions…")
			case cs.err != "":
				ui.Text(c, cs.err).FontSize(Typography().Body).TextColor(t.Warning)
			case !cs.sides.Editable:
				s.conflictWholeFile(c)
			default:
				s.conflictEditor(c)
			}
			ui.Row(c).FillWidth().Gap(Spacing().M).Justify(ui.End).AlignItems(ui.Center).Children(func() {
				if cs.sides.Editable && !cs.loading {
					s.conflictStatus(c)
					ui.Spacer(c)
				}
				if regularButton(c, "Cancel").Clicked() {
					cs.open = false
				}
				if cs.sides.Editable && !cs.loading {
					content := cs.result.Buffer.String()
					blocked := gitworkbench.HasConflictMarkers(content)
					apply := regularPrimaryButton(c, "Apply & Stage").Disabled(blocked || s.git.opBusy)
					if blocked {
						tipped(c, apply, "Decide every conflict first, or edit the markers out of the Result.")
					}
					if apply.Clicked() && !blocked {
						s.applyConflictResult(content)
					}
				}
			})
		})
	})
}

func (s *Shell) conflictStatus(c *ui.Context) {
	cs := &s.gitMore.conflict
	t := c.Theme()
	text := ""
	switch {
	case cs.edited():
		text = "Result edited by hand"
	case cs.undecided() > 0:
		text = itoa(cs.undecided()) + " of " + itoa(len(cs.hunks)) + " conflicts left"
	default:
		text = "All " + itoa(len(cs.hunks)) + " conflicts decided"
	}
	ui.Text(c, text).FontSize(Typography().BodySmall).TextColor(t.TextMuted).SingleLine()
}

// conflictWholeFile is what a file with no line-by-line merge offers.
func (s *Shell) conflictWholeFile(c *ui.Context) {
	cs := &s.gitMore.conflict
	t := c.Theme()
	ui.Column(c).Grow(1).MinHeight(0).Gap(Spacing().M).Children(func() {
		ui.Text(c, cs.sides.Reason).FontSize(Typography().Body).TextColor(t.TextMuted)
		ui.Row(c).Gap(Spacing().M).Children(func() {
			if regularButton(c, "Keep Current Branch's Version").Disabled(s.git.opBusy).Clicked() {
				cs.open = false
				s.resolveConflictSide(cs.path, gitworkbench.ConflictOurs, "Kept the current branch's version of ")
			}
			if regularButton(c, "Take Incoming Version").Disabled(s.git.opBusy).Clicked() {
				cs.open = false
				s.resolveConflictSide(cs.path, gitworkbench.ConflictTheirs, "Took the incoming version of ")
			}
		})
	})
}

func (s *Shell) conflictEditor(c *ui.Context) {
	cs := &s.gitMore.conflict
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(Spacing().M).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Choose what each conflict becomes; the Result below follows.").
			FontSize(Typography().BodySmall).TextColor(t.TextMuted).Grow(1).MinWidth(0)
		ui.Checkbox(c.Key("conflict-base"), &cs.showBase, "Show base")
	})
	ui.List(c, &cs.list, len(cs.rows), func(i int) {
		if i < 0 || i >= len(cs.rows) {
			return
		}
		s.conflictRowView(c, cs.rows[i])
	}).Grow(3).MinHeight(0)
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(Spacing().M).Children(func() {
		ui.Text(c, "Result").FontSize(Typography().BodySmall).FontWeight(WeightMedium)
		ui.Spacer(c)
		if cs.edited() && compactButton(c.Key("conflict-reset"), "Reset Result").Clicked() {
			cs.resetResult()
		}
	})
	cs.result.Theme = codeEditorTheme(t)
	ui.Column(c).FillWidth().Grow(2).MinHeight(120).Radius(Radius().Row).
		Border(1, designTokens(t.Dark).BorderSubtle).Clip().Children(func() {
		editor.View(c, cs.result).FillWidth().Grow(1).MinHeight(0)
	})
}

func (s *Shell) conflictRowView(c *ui.Context, row conflictRow) {
	t := c.Theme()
	pal := gdPaletteFor(t)
	switch {
	case row.hunk > 0:
		s.conflictHunkCard(c, row.hunk)
	case row.gap > 0:
		ui.Row(c).FillWidth().Padding(Spacing().XS, Spacing().M).Background(pal.gapBg).Children(func() {
			ui.Text(c, itoa(row.gap)+" unchanged lines").FontSize(Typography().Caption).TextColor(pal.gapText)
		})
	default:
		ui.Column(c).FillWidth().Padding(0, Spacing().M).Children(func() {
			conflictCode(c, row.lines, pal.muted, ui.Transparent)
		})
	}
}

// conflictCode draws a block of lines in the code font.
func conflictCode(c *ui.Context, text string, color, bg ui.Color) {
	shown, more := capLines(text, conflictShownLines)
	ui.Column(c).FillWidth().Background(bg).Padding(2, Spacing().S).Children(func() {
		ui.Text(c, strings.TrimSuffix(shown, "\n")).Font(gdCodeFont()).FontSize(Typography().BodySmall).TextColor(color)
		if more > 0 {
			ui.Text(c, "… "+itoa(more)+" more lines").FontSize(Typography().Caption).TextColor(color)
		}
	})
}

// capLines keeps the first n lines of text and counts the rest.
func capLines(text string, n int) (string, int) {
	lines := strings.SplitAfter(text, "\n")
	if k := len(lines); k > 0 && lines[k-1] == "" {
		lines = lines[:k-1]
	}
	if len(lines) <= n {
		return text, 0
	}
	return strings.Join(lines[:n], ""), len(lines) - n
}

func (s *Shell) conflictHunkCard(c *ui.Context, n int) {
	cs := &s.gitMore.conflict
	t := c.Theme()
	pal := gdPaletteFor(t)
	h := cs.hunks[n-1]
	choice := cs.choices[n-1]
	locked := cs.edited()
	key := "conflict-" + itoa(n) + "-"
	ui.Column(c).FillWidth().Radius(Radius().Row).Border(1, pal.cardBorder).Clip().Children(func() {
		ui.Row(c).FillWidth().Gap(Spacing().S).Padding(Spacing().XS, Spacing().M).AlignItems(ui.Center).Background(pal.headerBg).Children(func() {
			ui.Text(c, "Conflict "+itoa(n)+" of "+itoa(len(cs.hunks))).FontSize(Typography().BodySmall).FontWeight(WeightMedium)
			label, tone := conflictChoiceLabel(choice)
			statusPill(c, label, tone)
			ui.Spacer(c)
			pick := func(name string, ch gitworkbench.HunkChoice, tip string) {
				var b ui.Element
				if choice == ch {
					b = compactPrimaryButton(c.Key(key+name), name)
				} else {
					b = compactButton(c.Key(key+name), name)
				}
				b.Disabled(locked)
				tipped(c, b, tip)
				if b.Clicked() {
					cs.choose(n, ch)
				}
			}
			pick("Current", gitworkbench.ChoiceOurs, "Keep the current branch's lines")
			pick("Incoming", gitworkbench.ChoiceTheirs, "Take the incoming lines")
			pick("Both", gitworkbench.ChoiceBoth, "Current lines, then incoming lines")
			if cs.showBase {
				pick("Base", gitworkbench.ChoiceBase, "Go back to the common ancestor's lines")
			}
		})
		ui.Row(c).FillWidth().Children(func() {
			conflictSide(c, "Current", h.Ours, pal.addBg, pal.addBar, pal.code)
			conflictSide(c, "Incoming", h.Theirs, pal.selected, t.Accent, pal.code)
			if cs.showBase {
				conflictSide(c, "Base", h.Base, pal.emptySide, pal.muted, pal.muted)
			}
		})
	})
}

func conflictSide(c *ui.Context, title, text string, bg, bar, color ui.Color) {
	typ := Typography()
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).Background(bg).BorderWidth(0, 0, 0, 3).BorderColor(bar).Children(func() {
		ui.Text(c, title).FontSize(typ.Caption).FontWeight(WeightMedium).TextColor(color).Padding(Spacing().XS, Spacing().S, 0)
		if text == "" {
			ui.Text(c, "(no lines)").FontSize(typ.Caption).TextColor(color).Padding(2, Spacing().S, Spacing().XS)
			return
		}
		conflictCode(c, text, color, ui.Transparent)
	})
}

func conflictChoiceLabel(ch gitworkbench.HunkChoice) (string, StatusTone) {
	switch ch {
	case gitworkbench.ChoiceOurs:
		return "Current", ToneSuccess
	case gitworkbench.ChoiceTheirs:
		return "Incoming", ToneInfo
	case gitworkbench.ChoiceBoth:
		return "Both", ToneInfo
	case gitworkbench.ChoiceBase:
		return "Base", ToneNeutral
	}
	return "Undecided", ToneWarning
}

func (s *Shell) applyConflictResult(content string) {
	cs := &s.gitMore.conflict
	path := cs.path
	cs.open = false
	s.runGitOp("Resolve Conflict", func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.ResolveWith(ctx, root, path, content); err != nil {
			return "", err
		}
		return "Resolved " + path + " and staged it", nil
	})
}

// resolveConflictSide keeps one whole version of a conflicted file.
func (s *Shell) resolveConflictSide(path string, side gitworkbench.ConflictSide, done string) {
	s.runGitOp("Resolve Conflict", func(ctx context.Context, root string) (string, error) {
		if err := s.git.runner.ResolveConflict(ctx, root, path, side); err != nil {
			return "", err
		}
		return done + path, nil
	})
}

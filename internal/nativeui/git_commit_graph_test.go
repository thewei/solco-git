package nativeui

/**
 * [INPUT]: git_commit_graph.go、手写的提交序列（只有哈希与父）
 * [OUTPUT]: 路线图合同：布局（线性、分叉/合并、车道收拢、多尖端、父不在窗口内）、几何（槽宽有界、每行文字起点随该行用到的车道）、画法（线与点真的画在车道上、光晕盖住交叉线的缝）；设置 SOLCOGIT_EVIDENCE 时落 PNG
 * [POS]: TanGit / LazyGit 路线图移植的防线；每条都做过变异检查
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

func commitsOf(rows ...[]string) []gitworkbench.CommitInfo {
	out := make([]gitworkbench.CommitInfo, len(rows))
	for i, r := range rows {
		out[i] = gitworkbench.CommitInfo{Hash: r[0], Parents: r[1:]}
	}
	return out
}

// graphRowText draws a row as characters, for readable assertions: the dot,
// the verticals, and the corners where a line turns.
func graphRowText(row gitGraphRow) string {
	n := max(row.width, 1)
	up, down, left, right := make([]bool, n), make([]bool, n), make([]bool, n), make([]bool, n)
	for _, pipe := range row.pipes {
		if pipe.to == "" {
			continue // a root commit: nothing runs below it
		}
		if pipe.from == gitGraphStart && pipe.fromPos == pipe.toPos {
			continue // nothing runs above the first commit
		}
		switch pipe.kind {
		case gitPipeTerminates:
			up[pipe.fromPos] = true
		case gitPipeStarts:
			down[pipe.toPos] = true
		case gitPipeContinues:
			up[pipe.fromPos] = true
			down[pipe.toPos] = true
		}
		if pipe.fromPos < pipe.toPos {
			right[pipe.fromPos], left[pipe.toPos] = true, true
		} else if pipe.fromPos > pipe.toPos {
			left[pipe.fromPos], right[pipe.toPos] = true, true
		}
	}
	out := make([]rune, n)
	for i := range out {
		switch {
		case i == row.pos && row.merge:
			out[i] = '◎'
		case i == row.pos:
			out[i] = '●'
		case up[i] && down[i]:
			out[i] = '│'
		case up[i] && right[i]:
			out[i] = '╰'
		case up[i] && left[i]:
			out[i] = '╯'
		case down[i] && right[i]:
			out[i] = '╭'
		case down[i] && left[i]:
			out[i] = '╮'
		case left[i] && right[i]:
			out[i] = '─'
		case up[i]:
			out[i] = '╵'
		case down[i]:
			out[i] = '╷'
		default:
			out[i] = ' '
		}
	}
	return string(out)
}

func graphTexts(rows []gitGraphRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = graphRowText(r)
	}
	return out
}

func wantTexts(t *testing.T, rows []gitGraphRow, want ...string) {
	t.Helper()
	got := graphTexts(rows)
	if len(got) != len(want) {
		t.Fatalf("got %d rows %q, want %d %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d draws %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGraphLayoutLinearHistory(t *testing.T) {
	rows := gitHistoryGraph(commitsOf([]string{"c3", "c2"}, []string{"c2", "c1"}, []string{"c1"}))
	wantTexts(t, rows, "●", "●", "●")
	if n := gitGraphLaneCount(rows); n != 1 {
		t.Errorf("lanes = %d, want 1", n)
	}
}

func TestGraphLayoutForksAndMerges(t *testing.T) {
	rows := gitHistoryGraph(commitsOf(
		[]string{"merge", "a", "b"}, // the second parent opens a lane
		[]string{"a", "base"},
		[]string{"b", "base"},
		[]string{"base"}, // both lanes wait for it: it joins them back
	))
	wantTexts(t, rows, "◎╮", "●│", "│●", "●╯")
	if n := gitGraphLaneCount(rows); n != 2 {
		t.Errorf("lanes = %d, want 2", n)
	}
}

// A lane that ends closes up: once base has joined lane 1 back, the line on
// lane 2 moves into the free lane 1 on the next row, and the graph is two
// lanes wide again instead of staying three.
func TestGraphLayoutClosesUpFreedLanes(t *testing.T) {
	rows := gitHistoryGraph(commitsOf(
		[]string{"m", "x", "s1", "s2"}, []string{"x", "base"}, []string{"s1", "base"},
		[]string{"base", "b2"}, []string{"b2", "z"}, []string{"s2", "z"}, []string{"z"},
	))
	wantTexts(t, rows, "◎╮╮", "●││", "│●│", "●╯│", "●╭╯", "│●", "●╯")
}

// A second tip (a branch nothing above waits for) takes a lane of its own; a
// parent that is not in the window leaves its line running to the bottom.
func TestGraphLayoutSeveralTipsAndTruncatedWindow(t *testing.T) {
	rows := gitHistoryGraph(commitsOf(
		[]string{"tipA", "a1"},
		[]string{"tipB", "b1"},
		[]string{"a1", "gone"},
	))
	wantTexts(t, rows, "●", "│●", "●│")
	var runsOn bool
	for _, p := range rows[2].pipes {
		runsOn = runsOn || (p.from == "a1" && p.to == "gone" && p.kind == gitPipeStarts)
	}
	if !runsOn {
		t.Fatalf("a1 lost its line to the parent outside the window: %+v", rows[2].pipes)
	}
}

// A line keeps its colour from the commit that opened it down its whole length.
func TestGraphLaneColorFollowsTheBranch(t *testing.T) {
	rows := gitHistoryGraph(commitsOf(
		[]string{"m", "a", "b"}, []string{"a", "base"}, []string{"b", "base"}, []string{"base"},
	))
	colorOf := func(row gitGraphRow, from, to string) int {
		for _, p := range row.pipes {
			if p.from == from && p.to == to {
				return p.color
			}
		}
		t.Fatalf("no pipe %s→%s in %+v", from, to, row.pipes)
		return -1
	}
	opened := colorOf(rows[0], "m", "b")
	if got := colorOf(rows[1], "m", "b"); got != opened {
		t.Errorf("the side line changed colour crossing a row: %d → %d", opened, got)
	}
	if got := colorOf(rows[2], "b", "base"); got != opened {
		t.Errorf("the side commit's line is %d, want its branch's %d", got, opened)
	}
	if rows[0].color == opened {
		t.Errorf("the side branch %d shares the main line's colour", opened)
	}
}

func TestGraphWithoutCommits(t *testing.T) {
	if rows := gitHistoryGraph(nil); len(rows) != 0 {
		t.Fatalf("got %d rows for no commits", len(rows))
	}
	if n := gitGraphLaneCount(nil); n != 1 {
		t.Fatalf("lanes = %d, want 1", n)
	}
}

// The column never grows past the bound: a graph with more branches than fit
// compresses its lanes instead of pushing the subject off the row, and a row
// is only as wide as the lanes it touches.
func TestGraphGeometryIsBounded(t *testing.T) {
	if got, want := gitGraphPitch(10), float32(gitGraphLanePitch); got != want {
		t.Errorf("ten lanes: pitch = %v, want %v", got, want)
	}
	if got := gitGraphDotsWidth(40, 40); got > gitGraphGutterMax {
		t.Errorf("forty lanes: width %v exceeds the bound %v", got, gitGraphGutterMax)
	}
	if got := gitGraphDotsWidth(12, 12); got > gitGraphGutterMax {
		t.Errorf("twelve lanes: width %v exceeds the bound %v", got, gitGraphGutterMax)
	}
	if narrow, wide := gitGraphDotsWidth(8, 1), gitGraphDotsWidth(8, 8); !(narrow < wide) {
		t.Errorf("one lane's width %v is not left of eight lanes' %v", narrow, wide)
	}
	if a, b := gitGraphDotsWidth(0, 0), gitGraphDotsWidth(1, 1); a != b {
		t.Errorf("no lanes %v differs from one lane %v", a, b)
	}
}

// graphScene renders the glyph column of commits over paper, at one row each.
func graphScene(t *testing.T, commits []gitworkbench.CommitInfo, halo ui.Color) (*ui.Tester, []gitGraphRow, int) {
	t.Helper()
	rows := gitHistoryGraph(commits)
	lanes := gitGraphLaneCount(rows)
	width := int(gitGraphDotsWidth(lanes, lanes)) + 20
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Background(halo).Width(float32(width)).Height(float32(gitGraphRowH) * float32(len(rows))).Children(func() {
			for _, r := range rows {
				gitGraphGlyph(c, r, lanes, gitGraphRowWidth(r, lanes), halo, false)
			}
		})
	}, width, int(gitGraphRowH)*len(rows))
	tt.SetScale(2)
	tt.Frame()
	return tt, rows, width
}

func graphNear(a, b color.Color, tol int) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) int { return graphAbs(int(x>>8) - int(y>>8)) }
	return d(ar, br) <= tol && d(ag, bg) <= tol && d(ab, bb) <= tol
}

func graphAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func toStd(c ui.Color) color.Color {
	return color.NRGBA{c.R, c.G, c.B, c.A}
}

// The glyphs draw the graph where the layout put it: the dot of each row in
// its lane's colour, a line running on through a row that is not its own, and
// nothing at all where no lane is.
func TestGraphGlyphsPaintTheLanes(t *testing.T) {
	paper := ui.Hex("#ffffff")
	commits := commitsOf([]string{"m", "a", "b"}, []string{"a", "base"}, []string{"b", "base"}, []string{"base"})
	tt, rows, width := graphScene(t, commits, paper)
	img := tt.Image()
	sc := float64(img.Bounds().Dx()) / float64(width)
	lanes := gitGraphLaneCount(rows)
	pitch := float64(gitGraphPitch(lanes))
	at := func(lane, row int, dy float64) color.Color {
		x := float64(gitGraphLanePad) + (float64(lane)+0.5)*pitch
		y := float64(row)*float64(gitGraphRowH) + float64(gitGraphRowH)/2 + dy
		return img.At(int(x*sc), int(y*sc))
	}
	// Row 0: the merge's dot in lane 0, in lane 0's colour; row 1: lane 1's
	// line runs through (a is on lane 0), in the colour it was opened with.
	if got, want := at(0, 0, 0), toStd(gitGraphColor(rows[0].color)); !graphNear(got, want, 6) {
		t.Errorf("row 0's dot is %v, want %v", got, want)
	}
	if got, want := at(1, 1, 0), toStd(gitGraphColor(rows[0].pipes[len(rows[0].pipes)-1].color)); !graphNear(got, want, 6) {
		t.Errorf("lane 1 through row 1 is %v, want its branch's %v", got, want)
	}
	// Right of every lane the paper stays bare.
	if got := img.At(int((gitGraphRowWidth(rows[1], lanes)+6)*float32(sc)), int(1.5*float64(gitGraphRowH)*sc)); !graphNear(got, color.White, 0) {
		t.Errorf("past the lanes the paper is %v, want white", got)
	}
	// Row 3 joins the lanes back: lane 1 has nothing below the middle of the row.
	if got := at(1, 3, 8); !graphNear(got, color.White, 0) {
		t.Errorf("below the join, lane 1 is %v, want white", got)
	}
	saveEvidence(t, "graph-glyphs", tt)
}

// A real merge, read by the real reader: the Commits view lays it out as a
// fork and a join, two lanes wide, with the merge's dot marked as one, and the
// window renders it.
func TestCommitsViewDrawsARealMerge(t *testing.T) {
	root := gitFixture(t, "graph")
	gitRun(t, root, "stash", "-u", "-q")
	gitRun(t, root, "checkout", "-q", "feature")
	writeFile(t, root+"/side.txt", "side\n")
	gitRun(t, root, "add", ".")
	gitRun(t, root, "commit", "-q", "-m", "Side work")
	gitRun(t, root, "checkout", "-q", "main")
	gitRun(t, root, "merge", "--no-ff", "-q", "-m", "Merge feature", "feature")

	for _, dark := range []bool{true, false} {
		mode := map[bool]string{true: "dark", false: "light"}[dark]
		w, pump := newTestWindow(t)
		tt := newTester(w, 1280, 800, dark)
		sh := openAndLoad(t, w, pump, root)
		sh.showView(viewCommits)
		pump.until(t, "the commit list", func() bool { return len(sh.git.commits) >= 5 })
		settle(tt, pump, 4)

		view := sh.commitView()
		top := view.graphs[view.shown[0].Hash]
		if !top.merge || top.width != 2 {
			t.Fatalf("the merge row is %q (merge %v, width %d), want a two-lane merge", graphRowText(top), top.merge, top.width)
		}
		if view.lanes != 2 {
			t.Errorf("the history is %d lanes wide, want 2", view.lanes)
		}
		saveEvidence(t, "commits-merge-"+mode, tt)
	}
}

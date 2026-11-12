package nativeui

import (
	"cmp"
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

/**
 * [INPUT]: gitworkbench.CommitInfo 的哈希与父提交（日期序，父永远在子之后）、行的底色（给光晕用）
 * [OUTPUT]: gitHistoryGraph（每个提交一行的管道集布局：车道向左收拢）、gitGraphGlyph（VS Code 风格画法：两像素线 + 底色光晕、三次贝塞尔 S 弯、带环的圆点）、车道宽度与每行文字的起点
 * [POS]: Commits 列表的路线图：2026-11-12 起整体换成 TanGit / LazyGit 的画法（用户："整体换成 TanGit/LazyGit 风格，保留引用芯片与列，不留开关"），取代 Fork 式的直线车道
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 *
 * 布局是 LazyGit `pkg/gui/presentation/graph` 的 getNextPipes（MIT）经 TanGit
 * （github.com/FleetingEcho/TanGit，MIT，Copyright (c) 2026 FleetingEcho）移植到 MyGo 的版本；
 * LazyGit 的 Pipe 字段未导出，所以复制算法而不是 import。版权声明见 README.md 的第三方清单。
 */

// gitGraphStart marks the pipe that comes from above the first commit: it has
// no commit of its own, so there is no line above the graph's first row.
const gitGraphStart = "START"

// gitGraphPipeKind is how a pipe runs through one row.
type gitGraphPipeKind uint8

const (
	// gitPipeTerminates rises from the row's top edge to the commit's line.
	gitPipeTerminates gitGraphPipeKind = iota
	// gitPipeStarts falls from the commit's line to the row's bottom edge.
	gitPipeStarts
	// gitPipeContinues runs from the top edge to the bottom edge, moving left
	// or right where it crosses the commit's line.
	gitPipeContinues
)

// gitGraphPipe is one line of the graph through one row: from fromPos at the
// top edge, along the row's centre line, to toPos at the bottom edge,
// according to its kind. color follows the branch from the commit that opened
// it, so a line keeps its colour down its whole length.
type gitGraphPipe struct {
	fromPos, toPos int
	from, to       string
	kind           gitGraphPipeKind
	color          int
}

// gitGraphRow is the graph beside one commit: the pipes to paint, the lane the
// commit's dot sits in, and how many lanes this row touches.
type gitGraphRow struct {
	pipes []gitGraphPipe
	pos   int
	merge bool
	color int
	width int
}

// gitHistoryGraph assigns every commit a lane and draws the pipes between
// them, the way Git's graph does: a commit takes the lane waiting for it, or a
// new lane at the right, and lanes that free up are filled from the left by
// the lines still running, so the graph stays as narrow as it can.
//
// The commits arrive newest first in an order where a parent follows all of
// its children (the reader's date order), which is what lets one pass do it.
func gitHistoryGraph(commits []gitworkbench.CommitInfo) []gitGraphRow {
	if len(commits) == 0 {
		return nil
	}
	pipes := []gitGraphPipe{{from: gitGraphStart, to: commits[0].Hash, kind: gitPipeStarts}}
	rows := make([]gitGraphRow, 0, len(commits))
	for _, cm := range commits {
		pipes = gitGraphNextPipes(pipes, cm)
		rows = append(rows, gitGraphRowOf(pipes))
	}
	return rows
}

// gitGraphNextPipes runs the lanes through commit's row and returns the pipes
// that leave it towards the commits below.
func gitGraphNextPipes(prev []gitGraphPipe, commit gitworkbench.CommitInfo) []gitGraphPipe {
	// A pipe that terminated in the row above, or a root commit's pipe into
	// nothing, says nothing about this row.
	current := make([]gitGraphPipe, 0, len(prev))
	for _, pipe := range prev {
		if pipe.kind == gitPipeTerminates || pipe.to == "" {
			continue
		}
		current = append(current, pipe)
	}

	maxPos := -1
	for _, pipe := range current {
		maxPos = max(maxPos, pipe.toPos)
	}
	// A commit no line was waiting for takes a new lane at the far right; one a
	// line was waiting for takes that line's lane and colour.
	pos, color := maxPos+1, maxPos+1
	for _, pipe := range current {
		if pipe.to == commit.Hash {
			pos, color = pipe.toPos, pipe.color
			break
		}
	}

	// taken spots are those a line ends on, traversed ones those a line runs
	// through. A new pipe may not end on a spot a continuing pipe already runs
	// through, or it would read as that pipe's.
	taken := map[int]bool{}
	traversed := map[int]bool{}
	continuing := map[int]bool{}
	for _, pipe := range current {
		if pipe.to != commit.Hash {
			continuing[pipe.toPos] = true
		}
	}
	freeForContinuing := func() int {
		for i := 0; ; i++ {
			if !traversed[i] {
				return i
			}
		}
	}
	freeForNew := func() int {
		for i := 0; ; i++ {
			if !taken[i] && !continuing[i] {
				return i
			}
		}
	}
	traverse := func(from, to int) {
		left, right := from, to
		if left > right {
			left, right = right, left
		}
		for i := left; i <= right; i++ {
			traversed[i] = true
		}
		taken[to] = true
	}

	parent := ""
	if len(commit.Parents) > 0 {
		parent = commit.Parents[0]
	}
	pipes := make([]gitGraphPipe, 0, len(current)+len(commit.Parents))
	pipes = append(pipes, gitGraphPipe{fromPos: pos, toPos: pos, from: commit.Hash, to: parent, kind: gitPipeStarts, color: color})

	// Lines waiting for this commit end here, joined to its dot; lines on a
	// lane left of the dot continue on that lane.
	for _, pipe := range current {
		switch {
		case pipe.to == commit.Hash:
			pipes = append(pipes, gitGraphPipe{fromPos: pipe.toPos, toPos: pos, from: pipe.from, to: pipe.to, kind: gitPipeTerminates, color: pipe.color})
			traverse(pipe.toPos, pos)
		case pipe.toPos < pos:
			to := freeForContinuing()
			pipes = append(pipes, gitGraphPipe{fromPos: pipe.toPos, toPos: to, from: pipe.from, to: pipe.to, kind: gitPipeContinues, color: pipe.color})
			traverse(pipe.toPos, to)
		}
	}

	// Every parent past the first opens a lane of its own.
	for i, parent := range commit.Parents {
		if i == 0 {
			continue
		}
		to := freeForNew()
		pipes = append(pipes, gitGraphPipe{fromPos: pos, toPos: to, from: commit.Hash, to: parent, kind: gitPipeStarts, color: to})
		taken[to] = true
	}

	// Lines right of the dot move left into the free lanes, so the graph closes
	// up as branches end instead of keeping the width it once had.
	for _, pipe := range current {
		if pipe.to == commit.Hash || pipe.toPos <= pos {
			continue
		}
		last := pipe.toPos
		for i := pipe.toPos; i > pos; i-- {
			if taken[i] || traversed[i] {
				break
			}
			last = i
		}
		pipes = append(pipes, gitGraphPipe{fromPos: pipe.toPos, toPos: last, from: pipe.from, to: pipe.to, kind: gitPipeContinues, color: pipe.color})
		traverse(pipe.toPos, last)
	}

	// Left to right, so a line ending on a lane paints before one passing
	// through it.
	slices.SortFunc(pipes, func(a, b gitGraphPipe) int {
		if a.toPos != b.toPos {
			return cmp.Compare(a.toPos, b.toPos)
		}
		return cmp.Compare(a.kind, b.kind)
	})
	return pipes
}

// gitGraphRowOf reads the row's dot and width from its pipes.
func gitGraphRowOf(pipes []gitGraphPipe) gitGraphRow {
	row := gitGraphRow{width: 1}
	starts := 0
	for _, pipe := range pipes {
		switch pipe.kind {
		case gitPipeStarts:
			if starts == 0 {
				row.color = pipe.color
			}
			starts++
			row.pos = pipe.fromPos
		case gitPipeTerminates:
			row.pos = pipe.toPos
		}
		row.width = max(row.width, pipe.fromPos+1, pipe.toPos+1)
	}
	row.merge = starts > 1
	row.pipes = pipes
	return row
}

// gitGraphLaneCount is the widest row's lane count, which every row's lanes
// are spaced by so a lane keeps its column down the whole list.
func gitGraphLaneCount(rows []gitGraphRow) int {
	lanes := 1
	for _, row := range rows {
		lanes = max(lanes, row.width)
	}
	return lanes
}

// The graph's geometry: lanes sit gitGraphLanePitch apart, far enough that the
// halo around one lane leaves the next alone, and the gutter is never wider
// than gitGraphGutterMax, so a graph with more branches than that compresses
// its lanes instead of pushing the commit's text off the row.
const (
	gitGraphLanePitch = 18
	gitGraphLanePad   = 10
	gitGraphGutterMax = 260
	// gitGraphRowH is the glyph's height and, by being the tallest child of a
	// padding-free row, the Commits row pitch: adjacent glyphs tile edge to
	// edge, so the lines run unbroken across rows.
	gitGraphRowH float32 = 24
	// gitGraphBend is how much of the row a lane transition takes: VS Code
	// sweeps 80 % of it, so a branch leaves its lane in a long S rather than a
	// short elbow.
	gitGraphBend = 0.8
)

// gitGraphPitch is the distance between neighbouring lanes: the pitch above,
// or what the bound leaves when the graph is too busy for it.
func gitGraphPitch(lanes int) float32 {
	lanes = max(1, lanes)
	return min(float32(gitGraphLanePitch), (float32(gitGraphGutterMax)-2*gitGraphLanePad)/float32(lanes))
}

// gitGraphRowWidth is the width one row's glyph takes: just past the lanes the
// row itself touches, so a stretch of single-lane history keeps its subjects
// close to the dot and a row that fans out makes room for all of them.
func gitGraphRowWidth(row gitGraphRow, lanes int) float32 {
	return gitGraphDotsWidth(lanes, row.width)
}

// gitGraphDotsWidth is the width of a glyph that spans rowLanes lanes of a
// graph with lanes lanes in all.
func gitGraphDotsWidth(lanes, rowLanes int) float32 {
	return gitGraphLanePad + float32(max(1, rowLanes))*gitGraphPitch(lanes)
}

// gitGraphColor is the colour of a lane: a fixed cycle of mid-tone hues, so a
// lane reads the same in both themes and crossing lines stay tellable apart on
// either background.
func gitGraphColor(lane int) ui.Color {
	palette := [...]ui.Color{
		ui.Hex("#4f7cf7"), // blue
		ui.Hex("#d9822b"), // orange
		ui.Hex("#8b5cf6"), // violet
		ui.Hex("#2f9e6e"), // green
		ui.Hex("#d9468a"), // pink
		ui.Hex("#0f9bb0"), // teal
		ui.Hex("#dc4c4c"), // red
		ui.Hex("#8a7b2a"), // olive
	}
	lane %= len(palette)
	if lane < 0 {
		lane += len(palette)
	}
	return palette[lane]
}

// gitGraphGlyph draws one row of the graph in a column width wide. halo is the
// row's own background (the paper with the selection or hover wash on it): a
// wider stroke of it under every line separates crossing lanes and keeps the
// dot from touching the lines that join it. dotOnly draws the commit's dot
// alone — a filtered list is not a history, so wires between rows that are no
// longer neighbours would lead nowhere.
func gitGraphGlyph(c *ui.Context, row gitGraphRow, lanes int, width float32, halo ui.Color, dotOnly bool) {
	ui.Box(c).Width(width).Height(gitGraphRowH).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		sc := p.Scale()
		if sc <= 0 {
			sc = 1
		}
		// A line is a whole number of device pixels wide and centred so its
		// edges fall on pixel boundaries (an odd one on a pixel's centre): a
		// lane is the same weight wherever it sits instead of antialiasing to
		// thick in one place and thin in another (user report 2026-11-03).
		wirePx := max(float32(math.Round(float64(2*sc))), 1)
		odd := int(wirePx)%2 == 1
		snap := func(v float32) float32 {
			if odd {
				return (float32(math.Floor(float64(v*sc))) + 0.5) / sc
			}
			return float32(math.Round(float64(v*sc))) / sc
		}
		wireW := wirePx / sc
		pitch := gitGraphPitch(lanes)
		laneX := func(lane int) float32 { return snap(r.X + gitGraphLanePad + (float32(lane)+0.5)*pitch) }
		top := float32(math.Floor(float64(r.Y*sc))) / sc
		bottom := float32(math.Ceil(float64((r.Y+r.H)*sc))) / sc
		midY := snap(r.Y + r.H/2)

		pos := row.pos
		if dotOnly {
			laneX = func(int) float32 { return snap(r.X + gitGraphLanePad + 0.5*pitch) }
			pos = 0
		} else {
			path := &ui.Path{}
			for _, pipe := range row.pipes {
				if pipe.from == gitGraphStart && pipe.fromPos == pipe.toPos {
					continue
				}
				if pipe.kind == gitPipeStarts && pipe.to == "" {
					continue // a root commit: nothing runs below it
				}
				from, to := laneX(pipe.fromPos), laneX(pipe.toPos)
				// Butt caps, as VS Code draws its graph: a round cap reaches
				// half a stroke past the row's edge, so the halo under one
				// row's line would paint over the line the row above drew.
				// Reset restores the round cap, so it is set for each pipe.
				path.Reset().Cap(ui.ButtCap)
				switch pipe.kind {
				case gitPipeTerminates:
					path.MoveTo(from, top)
					if pipe.fromPos == pipe.toPos {
						path.LineTo(to, midY)
						break
					}
					d := (midY - top) * gitGraphBend
					path.CubeTo(from, top+d, to, midY-d, to, midY)
				case gitPipeStarts:
					if pipe.fromPos == pipe.toPos {
						path.MoveTo(from, midY).LineTo(to, bottom)
						break
					}
					d := (bottom - midY) * gitGraphBend
					path.MoveTo(from, midY).CubeTo(from, midY+d, to, bottom-d, to, bottom)
				case gitPipeContinues:
					if pipe.fromPos == pipe.toPos {
						path.MoveTo(from, top).LineTo(to, bottom)
						break
					}
					d := (bottom - top) * gitGraphBend
					path.MoveTo(from, top).CubeTo(from, top+d, to, bottom-d, to, bottom)
				}
				p.StrokePath(path, wireW+2, halo.Alpha(0.75))
				p.StrokePath(path, wireW, gitGraphColor(pipe.color))
			}
		}
		// The dot sits on top of the lines it joins, ringed by the halo.
		radius := min(4.5, max(3, pitch*0.28))
		cx := laneX(pos)
		disc := ui.Rect{X: cx - radius, Y: midY - radius, W: 2 * radius, H: 2 * radius}
		p.Fill(disc, gitGraphColor(row.color), radius)
		p.Stroke(disc, halo.Alpha(0.75), radius, 1)
	})
}

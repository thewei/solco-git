package nativeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/codehl"
)

// codeEditorTheme dresses the framework's code editor in the review's
// palette: its code color, line numbers and syntax classes, through the one
// table (codehl.KindClasses) the review's own coloring reads too. Kinds the
// review has no class for (variables, operators, punctuation) read as
// plain code, as in the review; links and inline code borrow the tag and
// string colors.
func codeEditorTheme(t *ui.Theme) *editor.Theme {
	th := editor.DefaultTheme(t)
	p := gdPaletteFor(t)
	th.Text = p.code
	th.LineNumber = p.lineNumber
	for _, group := range codehl.KindClasses {
		for _, kind := range group.Kinds {
			th.Tokens[kind].Color = p.syntax[group.Class]
		}
	}
	return th
}

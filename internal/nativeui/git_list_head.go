package nativeui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gitListHead is the one-row head of a Git list: the search button, then the
// list's summary. The search field shows beneath the row only once opened.
func (s *Shell) gitListHead(c *ui.Context, search *headerSearch, padX float32, summary func()) {
	k := gorexColorsOf(c.Theme().Dark)
	ui.Column(c).FillWidth().Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Height(32).Padding(0, padX).Gap(6).AlignItems(ui.Center).Children(func() {
			search.button(c, k)
			summary()
		})
		if *search.Open {
			ui.Row(c).FillWidth().Padding(0, padX, 6).Children(func() { search.field(c) })
		}
	})
}

// filteredCommits is the history the Commits search leaves: subject, author
// or a hash prefix match, within what the Git-side Filters left.
func (s *Shell) filteredCommits() []gitworkbench.CommitInfo {
	base := s.baseCommits()
	q := strings.ToLower(strings.TrimSpace(s.gitCommitFilter))
	if q == "" {
		return base
	}
	var entries []gitworkbench.CommitInfo
	for _, cm := range base {
		if strings.Contains(strings.ToLower(cm.Subject), q) ||
			strings.Contains(strings.ToLower(cm.Author), q) ||
			strings.HasPrefix(cm.Hash, q) {
			entries = append(entries, cm)
		}
	}
	return entries
}

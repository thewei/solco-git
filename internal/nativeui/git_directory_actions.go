package nativeui

import (
	"strings"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gitDirectoryPaths never passes a broad directory to Git. It resolves only
// the changed files from the requested pane, so a prefix like src does not
// accidentally include src-old or unrelated/untracked disk contents.
func gitDirectoryPaths(snap *gitworkbench.ChangesSnapshot, directory string, pane gdViewSide) []string {
	if snap == nil || directory == "" {
		return nil
	}
	prefix := strings.TrimSuffix(directory, "/") + "/"
	paths := make([]string, 0)
	for _, file := range snap.Files {
		if !strings.HasPrefix(file.Path, prefix) {
			continue
		}
		switch pane {
		case gdViewUnstaged:
			if file.Unstaged || file.Untracked {
				paths = append(paths, file.Path)
			}
		case gdViewStaged:
			if file.Staged {
				paths = append(paths, file.Path)
			}
		}
	}
	return paths
}

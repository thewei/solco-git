package nativeui

import (
	"strconv"
	"strings"

	"github.com/thewei/solco-gitkit/gitworkbench"
)

// gdHunkPatch writes a hunk as the unified diff shows it: its @@ header,
// then each line behind its marker.
func gdHunkPatch(h gitworkbench.Hunk) string {
	var b strings.Builder
	b.WriteString("@@ -" + gdRange(h.OldStart, h.OldLines) + " +" + gdRange(h.NewStart, h.NewLines) + " @@")
	if h.Section != "" {
		b.WriteString(" " + h.Section)
	}
	b.WriteByte('\n')
	for _, l := range h.Lines {
		switch l.Kind {
		case gitworkbench.KindAdd:
			b.WriteByte('+')
		case gitworkbench.KindDelete:
			b.WriteByte('-')
		default:
			b.WriteByte(' ')
		}
		b.WriteString(l.Text)
		b.WriteByte('\n')
		if l.NoNewline {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
	return b.String()
}

func gdRange(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(count)
}

// gdFilePatch writes a file's hunks under ---/+++ headers; "" when the file
// has no text hunks (binary, deferred, mode-only).
func gdFilePatch(f *gdFile) string {
	hunks := f.hunks()
	if len(hunks) == 0 {
		return ""
	}
	oldPath, newPath := "a/"+f.cf.Path, "b/"+f.cf.Path
	if f.cf.OldPath != "" {
		oldPath = "a/" + f.cf.OldPath
	}
	switch f.cf.Status {
	case gitworkbench.StatusAdded, gitworkbench.StatusUntracked:
		oldPath = "/dev/null"
	case gitworkbench.StatusDeleted:
		newPath = "/dev/null"
	}
	var b strings.Builder
	b.WriteString("--- " + oldPath + "\n+++ " + newPath + "\n")
	for _, h := range hunks {
		b.WriteString(gdHunkPatch(h))
	}
	return b.String()
}

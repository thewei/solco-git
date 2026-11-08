package nativeui

/**
 * [INPUT]: files_markdown.go（previewTier 路由、markdownPreview、链接分类、图片读取）、files_webview.go 的 AVIF/ICO 页、真实临时仓库、headless ui.Tester
 * [OUTPUT]: 2026-11-08 预览原生优先合同（与 Solco 同一份）：每种文件的分级归属、Markdown 原生渲染⇄源码切换且不重读（源码视图仍带对 HEAD 的槽边）、链接只放行该放行的、图片只读仓库里且有像素预算、主题随外观只换一次、AVIF/ICO 页只有几百字节；设置面板没有旧的 Rich file preview 开关；设置 SOLCOGIT_EVIDENCE 时落下 Markdown 渲染的 PNG
 * [POS]: Files 一节「预览原生优先」的回归防线；读取/分类语义由 solco-gitkit/filesview 测试覆盖，渲染本身由 MyGo ui/markdown 的测试覆盖
 * [PROTOCOL]: 变更时更新此头部，然后检查 CLAUDE.md
 */

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/thewei/solco-git/internal/codehl"
	"github.com/thewei/solco-gitkit/filesview"
)

// realTempDir is t.TempDir() with its symlinks resolved (macOS' /var is one),
// the form the project root takes in the shell.
func realTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeAt(t *testing.T, dir, rel string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// tinyPNG is a 2×2 opaque PNG.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// TestPlanPreviewTierRoutesEachFileToItsOwner pins the routing table: only
// what the native tier cannot show (AVIF, ICO) ever reaches a page, and
// Markdown renders only while formatting it stays cheap.
func TestPlanPreviewTierRoutesEachFileToItsOwner(t *testing.T) {
	const mib = 1 << 20
	for _, tc := range []struct {
		name string
		size int64
		want previewTier
	}{
		{"main.go", 100, tierText},
		{"notes.txt", 10 * mib, tierText},
		{"README.md", 100, tierMarkdown},
		{"README.markdown", markdownRenderCap, tierMarkdown},
		{"README.md", markdownRenderCap + 1, tierText}, // too big to format: it reads as source
		{"shot.png", 100, tierImage},
		{"shot.JPG", 100, tierImage},
		{"shot.jpeg", 100, tierImage},
		{"anim.gif", 100, tierImage},
		{"photo.webp", 100, tierImage},
		{"scan.bmp", 100, tierImage},
		{"logo.svg", 100, tierSVG},
		{"logo.svg", svgRenderCap, tierSVG},
		{"logo.svg", svgRenderCap + 1, tierText}, // too heavy to draw each frame: it reads as source
		{"logo.svg", 30 * mib, tierText},
		{"icon.ico", 100, tierWebImage},
		{"photo.avif", 100, tierWebImage},
		{"photo.avif", filesview.ImagePreviewCap, tierWebImage},
		{"photo.avif", filesview.ImagePreviewCap + 1, tierImageTooLarge},
	} {
		if got := planPreviewTier(tc.name, tc.size); got != tc.want {
			t.Errorf("%s (%d bytes): tier %v, want %v", tc.name, tc.size, got, tc.want)
		}
	}
}

// TestClassifyMarkdownLink: a link opens only what it names — a web address
// outside, a heading inside the page, a file inside the project — and
// everything else (other schemes, paths that leave the project, symlinks that
// do, files that are not there) does nothing.
func TestClassifyMarkdownLink(t *testing.T) {
	root := realTempDir(t)
	guide := writeAt(t, root, "docs/guide.md", []byte("# guide\n"))
	other := writeAt(t, root, "docs/other.md", []byte("# other\n"))
	spaced := writeAt(t, root, "docs/my doc.md", []byte("# spaced\n"))
	top := writeAt(t, root, "README.md", []byte("# top\n"))
	outside := writeAt(t, filepath.Dir(root), filepath.Base(root)+"-outside.md", []byte("secret\n"))
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(root, "docs", "escape.md")); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		dest   string
		kind   markdownLinkKind
		target string
	}{
		{"https://example.com/a?b=1#c", linkExternal, "https://example.com/a?b=1#c"},
		{"http://example.com", linkExternal, "http://example.com"},
		{"mailto:dev@example.com", linkExternal, "mailto:dev@example.com"},
		{"http://", linkNone, ""},
		{"javascript:alert(1)", linkNone, ""},
		{"file:///etc/hosts", linkNone, ""},
		{"data:text/html,hi", linkNone, ""},
		{"solco://reveal?tab=1", linkNone, ""},
		{"", linkNone, ""},
		{"#install", linkAnchor, "#install"},
		{"other.md", linkFile, other},
		{"./other.md#part", linkFile, other},
		{"other.md?raw=1", linkFile, other},
		{"my%20doc.md", linkFile, spaced},
		{"../README.md", linkFile, top},
		{"/README.md", linkFile, top}, // root-relative, as a repository's links read
		{"missing.md", linkNone, ""},
		{"../docs", linkNone, ""}, // a folder is not a file
		{"../../" + filepath.Base(root) + "-outside.md", linkNone, ""},
		{"escape.md", linkNone, ""}, // a symlink that leaves the project
		{"/etc/hosts", linkNone, ""},
	} {
		kind, target := classifyMarkdownLink(root, guide, tc.dest)
		if kind != tc.kind || target != tc.target {
			t.Errorf("%q: got (%v, %q), want (%v, %q)", tc.dest, kind, target, tc.kind, tc.target)
		}
	}
}

// TestWithinRootRefusesWhatItCannotResolve: the guard answers for the real
// location — symlinks followed — and a path it cannot resolve is not inside.
func TestWithinRootRefusesWhatItCannotResolve(t *testing.T) {
	root := realTempDir(t)
	inside := writeAt(t, root, "a/b.txt", []byte("x"))
	outside := writeAt(t, filepath.Dir(root), filepath.Base(root)+"-out.txt", []byte("x"))
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{
		root:                                 true,
		inside:                               true,
		filepath.Join(root, "a"):             true,
		filepath.Join(root, "missing.txt"):   false,
		filepath.Join(root, "link.txt"):      false,
		outside:                              false,
		filepath.Join(root, "a", "..", ".."): false,
		filepath.Join(root, "a", "..", "a"):  true,
		filepath.Join(root+"-sibling", "x"):  false,
	} {
		if got := withinRoot(root, path); got != want {
			t.Errorf("withinRoot(root, %q) = %v, want %v", strings.TrimPrefix(path, root), got, want)
		}
	}
	if withinRoot("", inside) || withinRoot(filepath.Join(root, "nope"), inside) {
		t.Error("an unresolvable root contains nothing")
	}
	// Neither side resolves: both read as "" and would "contain" each other.
	if withinRoot(filepath.Join(root, "nope"), filepath.Join(root, "nope", "deeper")) {
		t.Error("two paths that do not exist are not within one another")
	}
}

// bombPNG is a valid 1-bit grey PNG of w×h pixels whose all-zero rows
// compress to a few KB: cheap to store, expensive to decode — a decoder that
// trusts the header allocates w*h*4 bytes.
func bombPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	chunk := func(kind string, data []byte) {
		_ = binary.Write(&out, binary.BigEndian, uint32(len(data)))
		body := append([]byte(kind), data...)
		out.Write(body)
		_ = binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(body))
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(w))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(h))
	ihdr[8] = 1 // bit depth 1, colour type 0 (grey)
	chunk("IHDR", ihdr)
	var idat bytes.Buffer
	zw := zlib.NewWriter(&idat)
	row := make([]byte, 1+(w+7)/8) // filter byte 0 + zeros
	for i := 0; i < h; i++ {
		if _, err := zw.Write(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	chunk("IDAT", idat.Bytes())
	chunk("IEND", nil)
	return out.Bytes()
}

// TestMarkdownImageLoaderReadsOnlyProjectImagesWithinBudget: what a document
// says to show is untrusted, so the loader stays inside the project, refuses
// the network and anything past the byte and pixel budgets.
func TestMarkdownImageLoaderReadsOnlyProjectImagesWithinBudget(t *testing.T) {
	root := realTempDir(t)
	guide := writeAt(t, root, "docs/guide.md", []byte("# guide\n"))
	writeAt(t, root, "docs/pic.png", tinyPNG(t))
	writeAt(t, root, "shot.png", tinyPNG(t))
	writeAt(t, root, "docs/logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>`))
	writeAt(t, root, "docs/bomb.png", bombPNG(t, 8192, 8192))                                  // 67 Mpx over a few KB: past MaxNativeImagePixels
	writeAt(t, root, "docs/huge.png", append(tinyPNG(t), make([]byte, markdownImageCap+1)...)) // a valid PNG past the byte cap
	writeAt(t, root, "docs/notes.txt", []byte("not an image"))
	outside := writeAt(t, filepath.Dir(root), filepath.Base(root)+"-outside.png", tinyPNG(t))
	t.Cleanup(func() { _ = os.Remove(outside) })
	if err := os.Symlink(outside, filepath.Join(root, "docs", "escape.png")); err != nil {
		t.Fatal(err)
	}
	load := markdownImageLoader(root, guide)

	for _, ok := range []string{"pic.png", "./pic.png", "../shot.png", "/shot.png", "logo.svg"} {
		src, err := load(ok)
		if err != nil || src == nil {
			t.Errorf("%q must load: %v", ok, err)
		}
	}
	if src, _ := load("logo.svg"); src != nil {
		if _, isSVG := src.(*ui.SVG); !isSVG {
			t.Errorf("an SVG loads as a vector, got %T", src)
		}
	}
	for _, bad := range []string{
		"https://example.com/pic.png", "http://example.com/pic.png", "data:image/png;base64,AAAA", "file:///etc/hosts",
		"../../" + filepath.Base(root) + "-outside.png", "escape.png", "/etc/hosts",
		"bomb.png", "huge.png", "notes.txt", "missing.png", "..", "",
	} {
		if src, err := load(bad); err == nil || src != nil {
			t.Errorf("%q must not load (got %v, %v)", bad, src, err)
		}
	}
}

// TestMarkdownThemeFollowsTheReviewPalette: a code block colours a character
// as the review and the Files code view do, in both appearances, and the
// document reads at the section size with GitHub's rule under its H1/H2.
func TestMarkdownThemeFollowsTheReviewPalette(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := ui.LightTheme()
		if dark {
			theme = ui.DarkTheme()
		}
		th, p := markdownTheme(theme), gdPaletteFor(theme)
		if th.CodeColors == nil {
			t.Fatalf("dark=%v: code blocks need the review's colors", dark)
		}
		for kind, class := range map[editor.TokenKind]codehl.Class{
			editor.TokenKeyword: codehl.Keyword, editor.TokenString: codehl.String,
			editor.TokenComment: codehl.Comment, editor.TokenType: codehl.Type,
			editor.TokenNumber: codehl.Number,
		} {
			if got := th.CodeColors.Tokens[kind].Color; got != p.syntax[class] {
				t.Errorf("dark=%v kind %v = %v, want the review's %v", dark, kind, got, p.syntax[class])
			}
		}
		if th.FontSize != Typography().Section || th.CodeFont != gdCodeFont() {
			t.Errorf("dark=%v: type %v/%q, want the section size and the review's code font", dark, th.FontSize, th.CodeFont)
		}
		if !th.Headings[0].Rule || !th.Headings[1].Rule || th.Headings[2].Rule {
			t.Errorf("dark=%v: only H1 and H2 carry the rule: %+v", dark, th.Headings[:3])
		}
		if th.Text != (ui.Color{}) || th.Link != (ui.Color{}) {
			t.Errorf("dark=%v: text and link colors must stay the window theme's, which follows contrast and appearance", dark)
		}
	}
}

// TestMarkdownOptionsKeepOneThemePerAppearance: the view re-lays-out every
// paragraph when its theme value changes, so the theme must be the same
// pointer frame after frame and change only with the appearance.
func TestMarkdownOptionsKeepOneThemePerAppearance(t *testing.T) {
	mp := newMarkdownPreview("/p/a.md", "/p", "# a\n", func(string) {})
	light, dark := ui.LightTheme(), ui.DarkTheme()
	first := mp.options(light)
	lightTheme := first.Theme
	if lightTheme == nil {
		t.Fatal("the options must carry a theme")
	}
	if mp.options(light) != first || first.Theme != lightTheme {
		t.Fatal("a steady appearance must hand the view the same Options and the same Theme")
	}
	flipped := mp.options(dark)
	if flipped != first {
		t.Fatal("an appearance change must keep the Options: the image cache lives in them")
	}
	if flipped.Theme == nil || flipped.Theme == lightTheme {
		t.Fatal("an appearance change must swap the Theme")
	}
	if darkTheme := flipped.Theme; mp.options(dark).Theme != darkTheme {
		t.Fatal("the dark Theme must be as steady as the light one")
	}
}

func tableDoc() string {
	return "# Release notes\n\nSome **bold** words and a [link](https://example.com).\n\n" +
		"- first item\n- second item\n\n```go\nfunc main() {}\n```\n\n| name | size |\n|---|---|\n| alpha | 1 |\n"
}

// previewShell is a Files view on a fresh repository holding the given files.
func previewShell(t *testing.T, files map[string][]byte) (*Window, *uiPump, *Shell, *ui.Tester, string) {
	t.Helper()
	root := gitFixture(t, "preview")
	for rel, data := range files {
		writeAt(t, root, rel, data)
	}
	w, pump := newTestWindow(t)
	sh := openAndLoad(t, w, pump, root)
	sh.showView(viewFiles)
	tt := newTester(w, 1280, 800, false)
	settle(tt, pump, 3)
	return w, pump, sh, tt, root
}

// open selects path the way a tree click does and waits for its plan and read.
func openPreview(t *testing.T, sh *Shell, pump *uiPump, tt *ui.Tester, path string) {
	t.Helper()
	sh.openFilePreview(path)
	pump.until(t, "the plan of "+filepath.Base(path), func() bool {
		return sh.rightPanel.previewPlanned == path && !sh.rightPanel.nativeTextLoading && !sh.rightPanel.nativeImageLoading
	})
	settle(tt, pump, 3)
}

// TestMarkdownRendersAsFormattedTextAndSourceIsOneClickAway: a small Markdown
// file shows formatted, the Source choice swaps to the editor (with its gutter
// against HEAD) without reading the file again, and a file past the cap offers
// no Rendered at all.
func TestMarkdownRendersAsFormattedTextAndSourceIsOneClickAway(t *testing.T) {
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"notes.md": []byte(tableDoc()),
		"big.md":   bytes.Repeat([]byte("# x\n"), markdownRenderCap/4+1),
	})
	st := &sh.rightPanel
	md, big := filepath.Join(root, "notes.md"), filepath.Join(root, "big.md")

	openPreview(t, sh, pump, tt, md)
	if st.nativePreviewPath != md || st.nativeMarkdown == nil || st.nativeCode == nil {
		t.Fatalf("a small Markdown file holds both views: native=%q md=%v code=%v", st.nativePreviewPath, st.nativeMarkdown != nil, st.nativeCode != nil)
	}
	for _, want := range []string{"Rendered", "Source", "Release notes", "first item", "second item", "func main() {}", "alpha"} {
		if !tt.HasText(want) {
			t.Errorf("rendered view must show %q: %q", want, tt.Texts())
		}
	}
	for _, raw := range []string{"# Release notes", "**bold**", "```go", "|---|"} {
		if tt.HasText(raw) {
			t.Errorf("rendered view must not show the markup %q", raw)
		}
	}
	if tt.HasText("since HEAD") {
		t.Error("the code view's legend belongs to Source, not to the formatted text")
	}
	saveEvidence(t, "files-markdown-rendered", tt)

	code, rendered := st.nativeCode, st.nativeMarkdown
	sh.setPreviewSource(true)
	settle(tt, pump, 2)
	if st.nativeCode != code || st.nativeMarkdown != rendered {
		t.Fatal("switching to Source must keep both views, not read the file again")
	}
	if tt.HasText("first item") && !tt.HasText("- first item") {
		t.Fatalf("Source must show the markup, not the formatted text: %q", tt.Texts())
	}
	saveEvidence(t, "files-markdown-source", tt)
	sh.setPreviewSource(false)
	settle(tt, pump, 2)
	if !tt.HasText("first item") || tt.HasText("- first item") {
		t.Fatalf("Rendered must bring the formatted text back: %q", tt.Texts())
	}

	openPreview(t, sh, pump, tt, big)
	if st.nativePreviewPath != big || st.nativeMarkdown != nil || st.nativeCode == nil {
		t.Fatalf("an oversized Markdown file reads as source only: md=%v code=%v", st.nativeMarkdown != nil, st.nativeCode != nil)
	}
	if tt.HasText("Rendered") {
		t.Fatal("an oversized file cannot render, so it must not offer Rendered")
	}
}

// TestMarkdownSourceKeepsTheGitGutter: the formatted text has no gutter, but
// the Source of a tracked, modified Markdown file is still drawn against HEAD.
func TestMarkdownSourceKeepsTheGitGutter(t *testing.T) {
	_, pump, sh, tt, root := previewShell(t, nil)
	readme := filepath.Join(root, "README.md") // tracked and modified by the fixture
	st := &sh.rightPanel
	st.previewSource = true
	openPreview(t, sh, pump, tt, readme)
	if st.nativeCode == nil || st.nativeCode.DiffBase == nil {
		t.Fatalf("Source of a modified tracked file needs HEAD's text for the gutter: %+v", st.nativeCode)
	}
	if !tt.HasText("since HEAD") {
		t.Errorf("Source keeps the gutter's legend: %q", tt.Texts())
	}
}

// TestSearchHitInTheOpenRenderedFileLandsInSource: a content hit has a line to
// point at, which only the source has — so a hit in the Markdown file that is
// already open (formatted) flips it to Source and selects the match at once.
func TestSearchHitInTheOpenRenderedFileLandsInSource(t *testing.T) {
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{"notes.md": []byte(tableDoc())})
	md := filepath.Join(root, "notes.md")
	st := &sh.rightPanel
	openPreview(t, sh, pump, tt, md)
	if st.previewSource || st.nativeMarkdown == nil {
		t.Fatal("the file must be open formatted first")
	}
	sh.openFileAt(md, 4, 2, 5) // "- first item": the word "first"
	settle(tt, pump, 2)
	if !st.previewSource || st.reveal != nil {
		t.Fatalf("the hit must flip to Source and be consumed: source=%v reveal=%v", st.previewSource, st.reveal)
	}
	if got := st.nativeCode.SelectedText(); got != "first" {
		t.Fatalf("selected %q, want the match", got)
	}
}

// TestMarkdownLinksOpenWhatTheyNameAndNothingElse follows links the way a
// click does: web addresses go to the browser, a heading scrolls the page,
// a repository file opens in the preview (and its folders open in the tree),
// and the rest does nothing.
func TestMarkdownLinksOpenWhatTheyNameAndNothingElse(t *testing.T) {
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"docs/guide.md": []byte("# guide\n\n## Install\n"),
		"docs/other.md": []byte("# other\n"),
	})
	st := &sh.rightPanel
	guide, other := filepath.Join(root, "docs", "guide.md"), filepath.Join(root, "docs", "other.md")
	openPreview(t, sh, pump, tt, guide)
	mp := st.nativeMarkdown
	if mp == nil {
		t.Fatal("the guide must hold a rendered view")
	}
	var browser []string
	mp.openExternal = func(u string) { browser = append(browser, u) }

	mp.follow("https://example.com/docs")
	mp.follow("mailto:dev@example.com")
	mp.follow("javascript:alert(1)")
	mp.follow("file:///etc/hosts")
	if got := strings.Join(browser, " "); got != "https://example.com/docs mailto:dev@example.com" {
		t.Fatalf("browser opens = %q", got)
	}
	if st.selectedFilePath != guide {
		t.Fatalf("a refused link must not move the selection: %q", st.selectedFilePath)
	}

	version := mp.opts.AnchorVersion
	mp.follow("#install")
	mp.follow("#install")
	if mp.opts.Anchor != "#install" || mp.opts.AnchorVersion != version+2 {
		t.Fatalf("an anchor click scrolls, every click: %q v%d (was v%d)", mp.opts.Anchor, mp.opts.AnchorVersion, version)
	}

	mp.follow("other.md")
	pump.until(t, "the linked file's plan", func() bool { return st.previewPlanned == other })
	if st.selectedFilePath != other || st.nativePreviewPath != other {
		t.Fatalf("a repository link opens its file: selection %q", st.selectedFilePath)
	}
	if !st.expandedDirs[filepath.Dir(other)] {
		t.Error("the linked file's folder opens in the tree, so the selection is visible")
	}
}

// TestPreviewImagesGoNativeAndOnlyAVIFAndICOWantAPage: decodable formats are
// native bitmaps, an SVG is a native drawing, an oversized AVIF says so, and
// with no window to host the page an ICO says that instead of leaving the
// center blank.
func TestPreviewImagesGoNativeAndOnlyAVIFAndICOWantAPage(t *testing.T) {
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"dot.png":  tinyPNG(t),
		"logo.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`),
		"icon.ico": []byte("not decoded natively"),
		"big.avif": make([]byte, filesview.ImagePreviewCap+1),
		"big.svg":  make([]byte, svgRenderCap+1),
	})
	st := &sh.rightPanel
	at := func(name string) string { return filepath.Join(root, name) }

	openPreview(t, sh, pump, tt, at("dot.png"))
	if st.nativePreviewPath != at("dot.png") || st.nativeImageFailed != "" || st.webImage {
		t.Fatalf("a PNG is a native bitmap: native=%q failed=%q page=%v", st.nativePreviewPath, st.nativeImageFailed, st.webImage)
	}
	openPreview(t, sh, pump, tt, at("logo.svg"))
	if st.nativeSVG == nil || st.nativeCode == nil || st.webImage {
		t.Fatal("an SVG is a native drawing with its XML one click away, never a page")
	}
	openPreview(t, sh, pump, tt, at("big.svg"))
	if st.nativePreviewPath != at("big.svg") || st.nativeText == nil || st.nativeSVG != nil {
		t.Fatalf("an oversized SVG still reads as source: native=%q drawing=%v", st.nativePreviewPath, st.nativeSVG != nil)
	}
	openPreview(t, sh, pump, tt, at("big.avif"))
	if !strings.Contains(st.nativeImageFailed, "too large") {
		t.Fatalf("an oversized AVIF must say it is too large: %q", st.nativeImageFailed)
	}
	openPreview(t, sh, pump, tt, at("icon.ico"))
	if st.webImage || !strings.Contains(st.nativeImageFailed, "could not start") || !tt.HasText("Can't preview this image") {
		t.Fatalf("with no window to host the page the ICO must say so: page=%v failed=%q", st.webImage, st.nativeImageFailed)
	}
}

// TestSVGShowsAsADrawingAndSourceIsOneClickAway: an SVG opens as its drawing
// ("Image"), the Source choice swaps to the XML without reading the file
// again, and the choice is the one Markdown keeps. A file MyGo cannot parse
// has only its source, and says why.
func TestSVGShowsAsADrawingAndSourceIsOneClickAway(t *testing.T) {
	const logo = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#c00"/></svg>`
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"logo.svg":   []byte(logo),
		"broken.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect`),
	})
	st := &sh.rightPanel
	drawing, broken := filepath.Join(root, "logo.svg"), filepath.Join(root, "broken.svg")

	openPreview(t, sh, pump, tt, drawing)
	if st.nativeSVG == nil || st.nativeCode == nil || st.previewNote != "" {
		t.Fatalf("a drawable SVG holds both views: drawing=%v code=%v note=%q", st.nativeSVG != nil, st.nativeCode != nil, st.previewNote)
	}
	if !tt.HasText("Image") || !tt.HasText("Source") || tt.HasText("Rendered") {
		t.Fatalf("an SVG offers Image | Source, not Rendered: %q", tt.Texts())
	}
	if tt.HasText("since HEAD") {
		t.Error("the code view's legend belongs to Source, not to the drawing")
	}
	// The drawing is a red rectangle: pixels, not labels, tell it is on screen.
	if n := redPixels(tt.Image()); n < 2000 {
		t.Fatalf("the drawing view must paint the SVG: %d red pixels", n)
	}

	code, svg := st.nativeCode, st.nativeSVG
	sh.setPreviewSource(true)
	settle(tt, pump, 2)
	if st.nativeCode != code || st.nativeSVG != svg {
		t.Fatal("switching to Source must keep both views, not read the file again")
	}
	if n := redPixels(tt.Image()); n != 0 {
		t.Fatalf("Source is the XML, not the drawing: %d red pixels", n)
	}
	sh.setPreviewSource(false)
	settle(tt, pump, 2)
	if n := redPixels(tt.Image()); n < 2000 {
		t.Fatalf("Image must bring the drawing back: %d red pixels", n)
	}

	openPreview(t, sh, pump, tt, broken)
	if st.nativeSVG != nil || st.nativeCode == nil || st.previewNote == "" {
		t.Fatalf("an SVG that cannot be parsed has its source and a reason: drawing=%v code=%v note=%q", st.nativeSVG != nil, st.nativeCode != nil, st.previewNote)
	}
	if tt.HasText("Image") || !tt.HasText(st.previewNote) {
		t.Fatalf("the source shows with the reason and no Image choice: %q", tt.Texts())
	}
}

// redPixels counts the pixels of img that are the SVG test's red.
func redPixels(img *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if r, g, b := img.Pix[i], img.Pix[i+1], img.Pix[i+2]; r > 170 && g < 40 && b < 40 {
			n++
		}
	}
	return n
}

// TestMarkdownTooHeavyForAFrameReadsAsSourceAndSaysWhy: a block more than a
// frame can carry (a quote of thousands of paragraphs, a text past the engine's
// reach) cannot be formatted at all; the file shows its source with a note
// rather than freezing the window.
func TestMarkdownTooHeavyForAFrameReadsAsSourceAndSaysWhy(t *testing.T) {
	var quote strings.Builder
	for i := range mdBlockCap + 500 {
		quote.WriteString("> paragraph " + strings.Repeat("x", i%7) + "\n>\n")
	}
	huge := "# Title\n\n" + strings.Repeat("word ", mdLeafCap/5+10) + "\n"
	var many strings.Builder
	for i := range 4000 {
		many.WriteString("paragraph " + strings.Repeat("y", i%5) + "\n\n")
	}
	_, pump, sh, tt, root := previewShell(t, map[string][]byte{
		"quote.md":     []byte(quote.String()),
		"paragraph.md": []byte(huge),
		"many.md":      []byte(many.String()),
	})
	st := &sh.rightPanel
	for _, name := range []string{"quote.md", "paragraph.md"} {
		openPreview(t, sh, pump, tt, filepath.Join(root, name))
		if st.nativeMarkdown != nil || st.nativeCode == nil || st.previewNote == "" {
			t.Fatalf("%s: too heavy to format, so source and a reason: md=%v code=%v note=%q", name, st.nativeMarkdown != nil, st.nativeCode != nil, st.previewNote)
		}
		if tt.HasText("Rendered") || !tt.HasText(st.previewNote) {
			t.Fatalf("%s: no Rendered choice, the reason shown: %q", name, tt.Texts())
		}
	}
	// A big document that is only many ordinary blocks is not too heavy.
	openPreview(t, sh, pump, tt, filepath.Join(root, "many.md"))
	if st.nativeMarkdown == nil || st.nativeMarkdown.rows == nil || st.previewNote != "" {
		t.Fatalf("thousands of ordinary blocks format as rows: md=%v note=%q", st.nativeMarkdown != nil, st.previewNote)
	}
}

// TestSettingsOffersNoRichPreviewSwitch: the preview is native first and has
// nothing to turn off, so the retired switch is gone from Settings.
func TestSettingsOffersNoRichPreviewSwitch(t *testing.T) {
	root := gitFixture(t, "retired")
	w, pump := newTestWindow(t)
	_ = openAndLoad(t, w, pump, root)
	tt := newTester(w, 1280, 800, false)
	w.openSettings()
	settle(tt, pump, 3)
	if !tt.HasText("Auto-fetch") {
		t.Fatalf("the settings sheet did not render; texts: %q", tt.Texts())
	}
	if tt.HasText("Rich file preview") {
		t.Errorf("Settings still offers the retired switch; texts: %q", tt.Texts())
	}
}

// TestFilesWebviewFillIsTheWindowCanvas: the AVIF/ICO web view sits straight
// on the window canvas here (no card), so it is filled with the canvas role in
// both appearances and is made with that fill.
func TestFilesWebviewFillIsTheWindowCanvas(t *testing.T) {
	want := "light-dark(" + cssHexOf(designRoles(false).Canvas) + ", " + cssHexOf(designRoles(true).Canvas) + ")"
	if got := filesWebviewFill(); got != want {
		t.Fatalf("filesWebviewFill = %q, want the canvas pair %q", got, want)
	}
	src, err := os.ReadFile("files_webview.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "BackgroundColor: filesWebviewFill()") {
		t.Error("filesWebviewFor must fill its web view with filesWebviewFill()")
	}
}

func cssHexOf(c ui.Color) string {
	const digits = "0123456789abcdef"
	b := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		b[1+2*i], b[2+2*i] = digits[v>>4], digits[v&15]
	}
	return string(b)
}

// TestFilesImagePageIsAFewHundredBytes: the one page the preview ever loads
// is the image and a few hundred bytes around it — no script bundle, no
// stylesheet file, no network, no bridge.
func TestFilesImagePageIsAFewHundredBytes(t *testing.T) {
	url := "data:image/x-icon;base64,AAABAAEAEBAAAAEAIABoBAAAFgAAACgAAAAQ"
	page := filesImagePage(url)
	if !strings.Contains(page, `src="`+url+`"`) {
		t.Fatalf("the page must show the image it was given: %q", page)
	}
	if overhead := len(page) - len(url); overhead > 1024 {
		t.Fatalf("the page is %d bytes around the image; it must stay under a KB", overhead)
	}
	for _, banned := range []string{"<script src", "<link", "http://", "https://", "fetch(", "XMLHttpRequest", "mygo"} {
		if strings.Contains(page, banned) {
			t.Errorf("the page must not reach out: found %q", banned)
		}
	}
	if !strings.Contains(page, "onerror") {
		t.Fatal("an image the engine cannot decode must say so instead of showing a broken glyph")
	}
}

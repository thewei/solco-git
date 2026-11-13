package platform

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

// The first literal is also in Solco's TestParseTerminalURL: a change to the
// shape of the link breaks both.
func TestSolcoTerminalURL(t *testing.T) {
	cases := []struct {
		folder string
		want   string
		ok     bool
	}{
		{"/Users/me/app", "solco://terminal?cwd=%2FUsers%2Fme%2Fapp", true},
		{"/Users/me/app/../docs/", "solco://terminal?cwd=%2FUsers%2Fme%2Fdocs", true},
		{"/tmp/a b", "solco://terminal?cwd=%2Ftmp%2Fa+b", true},
		{"relative/path", "", false},
		{"", "", false},
		{"/tmp/x\x00y", "", false},
		{"/" + strings.Repeat("a", solcoMaxLink), "", false},
	}
	for _, c := range cases {
		got, ok := SolcoTerminalURL(c.folder)
		if ok != c.ok || got != c.want {
			t.Errorf("SolcoTerminalURL(%.40q) = %q, %v; want %q, %v", c.folder, got, ok, c.want, c.ok)
		}
	}
}

func testLink(goos string, present map[string]bool, env string) (*SolcoLink, *time.Time, *[]string) {
	clock := time.Unix(1000, 0)
	var ran []string
	l := &SolcoLink{
		goos: goos,
		now:  func() time.Time { return clock },
		stat: func(p string) (os.FileInfo, error) {
			if present[p] {
				return dirInfo{}, nil
			}
			return nil, fs.ErrNotExist
		},
		getenv: func(k string) string {
			if k == SolcoEnvApp {
				return env
			}
			return ""
		},
		home: func() (string, error) { return "/Users/me", nil },
		run:  func(app, link string) error { ran = append(ran, app+" "+link); return nil },
	}
	return l, &clock, &ran
}

func TestSolcoLinkFindsTheBundleInOrderAndCachesTheAnswer(t *testing.T) {
	present := map[string]bool{"/Applications/Solco.app": true, "/Users/me/Applications/Solco.app": true, "/dev/Solco.app": true}
	l, clock, _ := testLink("darwin", present, "/dev/Solco.app")
	if got := l.Path(); got != "/dev/Solco.app" {
		t.Fatalf("the override wins: %q", got)
	}
	l, clock, _ = testLink("darwin", present, "")
	if got := l.Path(); got != "/Applications/Solco.app" {
		t.Fatalf("/Applications before ~/Applications: %q", got)
	}
	delete(present, "/Applications/Solco.app")
	if got := l.Path(); got != "/Applications/Solco.app" {
		t.Errorf("the answer is cached within %v: %q", solcoRecheckEvery, got)
	}
	*clock = clock.Add(solcoRecheckEvery)
	if got := l.Path(); got != "/Users/me/Applications/Solco.app" {
		t.Errorf("after the cache lapses it looks again: %q", got)
	}
	delete(present, "/Users/me/Applications/Solco.app")
	*clock = clock.Add(solcoRecheckEvery)
	if l.Available() {
		t.Error("nothing found means unavailable")
	}
}

func TestSolcoLinkOnlyOnMacAndOnlyAnAppDirectory(t *testing.T) {
	l, _, _ := testLink("linux", map[string]bool{"/Applications/Solco.app": true}, "")
	if l.Available() {
		t.Error("Solco is a macOS app")
	}
	l, _, _ = testLink("darwin", map[string]bool{"/dev/Solco": true}, "/dev/Solco")
	if l.Available() {
		t.Error("an override that is not a .app directory is not a bundle")
	}
	var nilLink *SolcoLink
	if nilLink.Available() {
		t.Error("no link, no Solco")
	}
}

func TestSolcoLinkOpensTheTerminalInThatBundleOrRefuses(t *testing.T) {
	l, _, ran := testLink("darwin", map[string]bool{"/Applications/Solco.app": true}, "")
	if err := l.OpenTerminal("/Users/me/app"); err != nil {
		t.Fatal(err)
	}
	if want := "/Applications/Solco.app solco://terminal?cwd=%2FUsers%2Fme%2Fapp"; len(*ran) != 1 || (*ran)[0] != want {
		t.Errorf("ran %q, want %q", *ran, want)
	}
	if err := l.OpenTerminal("relative"); !errors.Is(err, ErrSolcoBadFolder) || len(*ran) != 1 {
		t.Errorf("a folder that cannot be a link is refused before anything runs: %v %q", err, *ran)
	}
	none, _, ran2 := testLink("darwin", nil, "")
	if err := none.OpenTerminal("/Users/me/app"); !errors.Is(err, ErrSolcoNotInstalled) || len(*ran2) != 0 {
		t.Errorf("not installed is refused first: %v %q", err, *ran2)
	}
}

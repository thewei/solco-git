module github.com/thewei/solco-git

go 1.27.3

tool github.com/egoist/mygo/cmd/mygo

// SHARED GIT / FILES DOMAIN (2026-11-07 Solco Git decision): gitworkbench and
// filesview live in the public repository github.com/thewei/solco-gitkit,
// required at a tagged version so CI needs no relative replace. On a machine
// with the repositories side by side, an uncommitted go.work (go work init . &&
// go work edit -replace github.com/thewei/solco-gitkit=../solco-gitkit) makes
// the local copy win; a gitkit change reaches this app by a new tag and a
// go get here (GOWORK=off).

require (
	github.com/egoist/mygo v0.8.1
	github.com/go-tex/math v0.50.0
	github.com/sourcegraph/jsonrpc2 v0.2.3
	github.com/thewei/solco-gitkit v0.3.0
	github.com/thewei/solco-gitkit/engine v0.1.0
	golang.org/x/net v0.59.0
	tuios.dev/sketch v0.4.0
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/adrg/xdg v0.5.3 // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/buger/jsonparser v1.1.2 // indirect
	github.com/cli/go-gh/v2 v2.16.2 // indirect
	github.com/cli/safeexec v1.0.1 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v3 v3.6.0 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-opentype/opentype v0.13.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gookit/color v1.6.1 // indirect
	github.com/jesseduffield/generics v0.0.0-20250517122708-b0b4a53a6f5c // indirect
	github.com/jesseduffield/lazygit v0.66.4-0.20261110130621-8290cc9357ad // indirect
	github.com/karimkhaleel/jsonschema v0.0.0-20231001195015-d933f0d94ea3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mgutz/str v1.2.0 // indirect
	github.com/petermattis/goid v0.0.0-20250813065127-a731cc31b4fe // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.16.0 // indirect
	github.com/sahilm/fuzzy v0.1.3 // indirect
	github.com/samber/lo v1.53.0 // indirect
	github.com/sasha-s/go-deadlock v0.3.9 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/stefanhaller/git-todo-parser v0.0.7-0.20250905083220-c50528f08304 // indirect
	github.com/wk8/go-ordered-map/v2 v2.1.8 // indirect
	github.com/xo/terminfo v1.2.0 // indirect
	golang.org/x/exp v0.0.0-20240719175910-8a7402abbf56 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/term v0.46.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

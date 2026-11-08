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
	github.com/thewei/solco-gitkit v0.2.0
)

require (
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

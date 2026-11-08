// Solco Git app configuration. A native-only MyGo app (no frontend yet), so
// `go tool mygo build` packages build/darwin-arm64/Solco Git.app straight from
// the Go program. The URL scheme is Solco's one-way link into this app:
// solcogit://open?repo=&view=&file= (see internal/nativeui/external_open.go).
export default {
  name: "Solco Git",
  identifier: "com.whstudio.solcogit",
  version: "0.2.0",
  urlSchemes: ["solcogit"],
  out: "build",
  icon: "resources/icon.png",

  // Signed updates (docs/releasing.md): installed copies check
  // thewei/solco-git's GitHub Releases once a day and on Check for Updates…,
  // and install only what the secret key behind this public key signed.
  updates: {
    publicKey: "JD43QkJP1d+Og1clRmHIyxaRbfpc1tTDhYHVinafv0A=",
    github: "thewei/solco-git",
  },

  macos: {
    minimumSystemVersion: "13.0",
  },
};

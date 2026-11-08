# site/
> L2 | Parent: /CLAUDE.md

index.html: the Solco Git landing page (deployed to https://thewei.github.io/solco-git/ by `.github/workflows/pages.yml`). A hero, one real full-window screenshot, three notes (Changes / Commits / Files), a README-style walkthrough with the Solco Git scope ("small on purpose"), the macOS download section and a four-column footer. Both Download CTAs resolve to the newest release: the static href is `releases/latest`, and a small script fetches `releases/latest` from the GitHub API (1 h localStorage cache), stamps the visible version, picks the DMG for the visitor's architecture and fills every download link and size from the release — never hardcode a versioned asset filename, release asset names carry the version.
styles.css: Solco's landing design tokens and presentation (the dark palette, silver pill actions, 120px section rhythm, README-style doc column), scoped under `.sg`, plus the Solco Git additions at the end (`.shot` whole-window screenshots, a one-group download grid). No framework dependencies.
assets/appicon/: the app icon exports (`solco-git-128/192/512.png`, `favicon-128.png`), resized from `resources/icon.png`.
assets/screens/: real screenshots of the app on a throwaway repository (`product-hero` Changes, `feature-commits`, `feature-files`), 1280×800 WebP.
CLAUDE.md: this file.

[PROTOCOL]: Update this header on change, then check CLAUDE.md.

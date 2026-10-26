# Markdown Browser

A small desktop browser for reading markdown. It asks servers for markdown, renders it, and falls back to a
title plus link list when a page is not markdown. Go backend, HTML/CSS/JS frontend, built with
[Wails](https://wails.io) so it runs on macOS, Windows and Linux with the system webview.

## Features

- Address bar, back, forward, reload, and a history dropdown (saved between runs)
- Requests `Accept: text/markdown`; renders markdown (GFM tables, task lists, footnotes) as sanitized HTML
- **Source** button shows the raw markdown
- Not markdown? Shows the page title and the links found on it
- RSS and Atom feeds render as a list of posts; an item's `<source:markdown>` is used when present
- `http`, `https` and `file://`; drop a `.md` file on the window to open it. Relative links and images work,
  including local images for pages opened from disk
- **Full history…** in the history menu generates a markdown page of everything you visited
- A network console (hidden by default; `>_` button or Cmd/Ctrl+J) lists every request with its status code;
  click the bar to see request and response headers, timing and size. Cookie and token values are hidden
- Bookmarks with folders, descriptions and keywords: see below
- Light and dark themes: follows the OS by default; the ◐ toolbar button cycles System, Light and Dark
- Sites that need a login: see below

| Shortcut | Action |
|---|---|
| Cmd/Ctrl+L | Focus the address bar |
| Cmd/Ctrl+R | Reload |
| Alt+← / Alt+→ (Cmd+[ / Cmd+] on Mac) | Back / forward |
| Cmd/Ctrl+U | Toggle markdown source |
| Cmd/Ctrl+D | Add or edit the bookmark for this page |
| Cmd/Ctrl+B | Open the bookmarks panel and move into it (again to close it) |
| Cmd/Ctrl+J | Show / hide the network console |

## Bookmarks

- The ribbon next to the address bar saves the current page (filled when it is already saved). The dialog takes a
  title, address, description, keywords and a folder. The panel button at the far left (Cmd/Ctrl+B) shows folders
  and bookmarks as a tree; the pencil on a row edits or deletes it, and **New folder** adds folders.
  In the panel, use ↑/↓ to move, → / ← to open and close folders, Enter to open a bookmark, **e** to edit and
  Esc to go back to the address bar.
- **Search** by typing in the address bar: several words (`rust async`), a single bare word (`rust`), or anything
  starting with `?` (`?go.dev`; a bare `?` lists everything). Every word must match the title, address,
  description or a keyword. Pages from your browsing history that match (title or address) are merged in
  after the bookmarks, without repeating a bookmarked page. Anything with a dot, colon or slash, like `example.com`, is an address instead.
- **Import / Export** (bottom of the panel) read and write browser-compatible HTML bookmark files (Chrome, Firefox,
  Safari and others) and markdown lists. Importing merges folders by name and leaves pages you already saved alone.
  Markdown exports look like this, and hand-written files in the same shape import too (headings also work as
  folders):

  ```markdown
  - **Dev**
    - [Go](<https://go.dev/>) — The Go site
      Keywords: lang, go
  ```
- **Where bookmarks are kept** (the ⚙ in the panel header): by default a local SQLite file, `bookmarks.db` in the
  app's config folder, or a folder of your [LinkaGoGo](https://www.linkagogo.com) account. For LinkaGoGo, create
  an API key under Account Settings → API Keys (Plus or Premium plan), paste it in the dialog, load your folders and
  pick the one to use as this app's top level, or let it create one. Only that folder and what is inside it is ever
  shown, searched or changed. The key goes to your system keychain, never to a file. Switching does not move
  bookmarks: export, switch, then import. The choice is saved in `settings.json` next to the database
  (`{"bookmarks": {"backend": "linkagogo", "config": {"root": "<folder id>"}}}`), and further backends plug in
  through `internal/bookmarks`.

## Sites that need a login

Click the key button (🔑), or the button on a 401/403 page, and add a credential for a site:

- **Cookies**: paste the `Cookie` request header from your browser's dev tools, or import a `cookies.txt`
  export (any "cookies.txt" browser extension) for that site. Only the cookies for the site you enter are kept.
- **Bearer token** or **username & password** for sites with API tokens or basic auth.

How credentials are handled:

- They are stored in the OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service) and
  never shown again or sent back to the UI.
- They are sent only to the exact origin (scheme, host and port) they were saved for, and are dropped on
  redirects to another origin.
- They are refused for plain `http` except for `localhost` and loopback addresses.
- Cookies a site sets are kept between runs in `cookies.bin`, encrypted with a key held in the keychain.
  "Clear saved cookies" removes them.
- The app does **not** read cookies out of other browsers.

Files live in the user config directory (`~/Library/Application Support/markdown-browser` on macOS,
`%AppData%\markdown-browser` on Windows, `~/.config/markdown-browser` on Linux): `history.json`, `history.md`,
`credentials.json` (site names only, no secrets) and `cookies.bin`.

## Build

Requirements: Go 1.25+, Node (for the frontend build), the [Wails CLI](https://wails.io/docs/gettingstarted/installation)
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

```sh
make dev       # live-reload development window
make test      # go vet + go test
make build     # app for the current OS, in build/bin
make windows   # cross-compile markdown-browser.exe (works from macOS/Linux)
```

Linux needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev` (and a Secret Service such as gnome-keyring for
credentials); build with `make linux`. A GitHub Actions workflow builds all three platforms.

## Releases

Push a version tag and GitHub Actions builds macOS (arm64), Windows and Linux, then publishes them on the
repository's Releases page:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The macOS build is signed and notarized. The Windows build is unsigned: choose More info, then Run anyway.

### Signing releases (macOS)

Locally, with a "Developer ID Application" certificate in your keychain:

```sh
xcrun notarytool store-credentials markdown-browser --apple-id <id> --team-id <team>   # once
make notarize
```

For the release workflow, add these repository secrets (Settings > Secrets and variables > Actions):

| Secret | Value |
| --- | --- |
| `MACOS_CERT_P12` | the Developer ID Application certificate exported as `.p12`, base64-encoded (`base64 -i cert.p12 \| pbcopy`) |
| `MACOS_CERT_PASSWORD` | the password you set when exporting the `.p12` |
| `APPLE_ID` | your Apple ID email |
| `APPLE_APP_PASSWORD` | an app-specific password from appleid.apple.com |
| `APPLE_TEAM_ID` | your 10-character team ID |

## Roadmap

Planned, in this order:

1. **Tabs.** Several pages open at once, each with its own back/forward history, with Cmd/Ctrl+T, W and Tab to open, close and switch.
2. **AI assistance.** Summarize the current page, and suggest a bookmark's description and keywords. Page text is sent to the chosen provider, so the app will say which one is active and ask before the first cloud call. Providers are pluggable, like bookmark storage, and API keys live in the OS keychain:
   - **Claude** (Anthropic Messages API), first.
   - **OpenAI-compatible** endpoints with a configurable base URL, which covers OpenAI, Gemini, Llama hosts and local servers such as Ollama.
   - **Apple on-device models** (macOS) through `fm serve`, which exposes them at `http://127.0.0.1:1976/v1/chat/completions`. It uses the same OpenAI-compatible provider, so no native code is needed and nothing leaves the machine.

## Layout

```
main.go, app.go        Wails bootstrap and the methods exposed to the UI
internal/fetch         HTTP/file loading, content negotiation, size and redirect limits
internal/render        markdown -> sanitized HTML, feeds, link extraction
internal/browser       ties fetch + render + history together
internal/nav           back/forward stack
internal/history       visit log and the generated history page
internal/bookmarks     bookmark model, Store interface, backends (sqlite/, linkagogo/, memory), conformance tests
internal/settings      settings.json (bookmark backend choice)
internal/auth          per-site credentials in the keychain
internal/cookies       encrypted persistent cookie jar
internal/localfile     serves local images to pages opened from disk
tools/lggcheck         read-only probe of the LinkaGoGo API with your key (prints shapes, not data)
frontend/              vanilla HTML/CSS/JS (no framework)
```

## License

[MIT](LICENSE)

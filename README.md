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
- Tabs, each with its own back/forward history; open tabs are restored on the next start (only the selected one is loaded right away). Cmd/Ctrl+click or middle-click a link to open it in a background tab; click a bookmark with Cmd/Ctrl, or right-click it, to open it in a new tab.
- A network console (hidden by default; `>_` button or Cmd/Ctrl+J) lists every request with its status code;
  click the bar to see request and response headers, timing and size. Cookie and token values are hidden
- Bookmarks with folders, descriptions and keywords: see below
- The ☰ menu at the far right holds Site credentials, the theme (follow the OS by default, or Light or Dark) and the color scheme: Violet (default), Ocean, Forest, Sunset, Rose or Graphite
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
| Cmd/Ctrl+Shift+A | Show / hide the Ask this page panel |
| Cmd/Ctrl+T, Cmd/Ctrl+W | New tab, close tab |
| Ctrl+Tab / Ctrl+Shift+Tab, Cmd/Ctrl+Shift+[ / ], Cmd/Ctrl+1–9 | Switch tabs (9 is the last) |

## Bookmarks

- The ribbon next to the address bar saves the current page (filled when it is already saved). The dialog takes a
  title, address, description, keywords and a folder. The panel button at the far left (Cmd/Ctrl+B) shows folders
  and bookmarks as a tree; the pencil on a row edits or deletes it, and **New folder** adds folders. Drag a bookmark or folder onto a folder to move it, or onto empty space for the top level (Esc cancels).
  In the panel, use ↑/↓ to move, → / ← to open and close folders, Enter to open a bookmark, **e** to edit and
  Esc to go back to the address bar.
- **Search** by typing in the address bar: several words (`rust async`), a single bare word (`rust`), or anything
  starting with `?` (`?go.dev`; a bare `?` lists everything). Every word must match the title, address,
  description or a keyword. Pages from your browsing history that match (title or address) are merged in
  after the bookmarks, without repeating a bookmarked page. Anything with a dot, colon or slash, like `example.com`, is an address instead.
- **Search by meaning** starts with `@` (`@that article about async rust I read last month`) and needs the AI assistant (see below).
- **Import / Export** (bottom of the panel) read and write browser-compatible HTML bookmark files (Chrome, Firefox,
  Safari and others) and markdown lists. Importing merges folders by name and skips a page that is already in the same folder (the same page in another folder is imported).
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

## AI assistant

The sparkle button (✨) in the toolbar summarizes the current page in a card above it, and **Suggest from the page**
in the bookmark dialog fills in an empty title, description and keywords for you to edit. Choose a provider under
☰ → **AI assistant**; the first click on either button opens that dialog if nothing is set up.

Any server that speaks the OpenAI chat-completions API works. Presets are filled in for Claude (Anthropic's
compatible endpoint), OpenAI, Google Gemini, Ollama, and Apple's on-device model through `fm serve`
(`http://127.0.0.1:1976/v1`; run `fm serve` first). Or enter your own address and model.

The speech-bubble button (💬, Cmd/Ctrl+Shift+A) opens **Ask this page**: a conversation about the page in the current tab, in a
panel on the right. Each tab has its own conversation, about one page. It is saved after every answer and comes back when you open
that page again, also after a restart (**Clear** deletes it; ☰ → AI assistant → *Delete saved conversations* deletes them all). The text
of your questions and the answers is kept in `conversations.json` in the config folder, readable only by you, for up to 50 pages. Answers are rendered as markdown, with links that open in the browser, and show the model and how long it took.
Enter sends and Shift+Enter starts a new line. While a model is working, the **Cancel** button (or Esc) stops it, and so do the Cancel buttons on the summary card and the bookmark suggestion; the question goes back into the box.

**Tag with AI** (a button in the bookmarks panel footer, or right-click a folder to start with just that folder) asks the AI for a short
title, a description and keywords for many bookmarks at once. You choose the folder and whether to cover only bookmarks that are
missing a title, description or keywords, or every bookmark (new descriptions and keywords replace the old ones). Existing titles are
kept unless you tick *Replace existing titles with short AI titles*. The dialog first says how many pages will be read and the most text that could be
sent. Pages are read two at a time, **Stop** ends the run early, and a rate-limit error from the provider stops it too. Then
a table shows every suggestion with an editable title, description and keywords: nothing is saved until you untick what you do not want and
press **Apply**. Pages that could not be read are listed at the end with the reason.

**Search by meaning** is typed in the address bar with a leading `@`: `@that article about async rust I read last month`. Plain
searches (`rust async`, `?rust`) still need every word to match and never use the AI. The `@` search offers the model your bookmarks and recent
browsing history (up to 400 entries, the most recent ones, and those sharing words with your query first when you have more) and shows the ones it picks, best first, each with the
model's reason. Unlike the other features it sends not the text of a page but the **title, address, folder, keywords and description of each
bookmark and history entry**, so it needs the same permission for servers outside your computer. The results page says what was sent and where. A
keyword results page has a button to rerun the search by meaning, and the other way round. Searches by meaning are not recorded in history.

**Sort unfiled** (a button in the bookmarks panel footer) asks the AI where the bookmarks that sit in no folder belong. It prefers your
existing folders and proposes a new one only when nothing fits. It sends the title, address, keywords and description of each unfiled
bookmark and the names of your folders (50 bookmarks per request), not the pages. You review a table, change any folder (typing a new
path creates it), untick what you do not want, and only then are bookmarks moved.

- The API key is kept in the system keychain, never in `settings.json`.
- A summary, suggestion or question sends the page's text (cut to about 30,000 characters) to the provider. For a server that is not on this
  computer you must tick *Allow sending page text to …* once; servers on this computer need no permission.
- Pages are re-fetched for this, with your saved credentials, so pages behind a login work. Model output is shown
  as sanitized markdown, and the prompts tell the model to ignore instructions inside the page.
- The model can be wrong; the summary card says so.

## Sites that need a login

Open **Site credentials** in the ☰ menu (it is highlighted when the current site has one saved), or use the button on a 401/403 page, and add a credential for a site:

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
credentials); build with `make linux`.

The Linux release is a folder with the program, `install.sh`, a launcher entry and the icon. Run `./install.sh` inside it to put them under
`~/.local` (program, application-menu entry and icon; delete those three files to uninstall). Without it the program runs but
Wayland desktops have no launcher to take the taskbar icon from; X11 desktops show the window icon either way. A GitHub Actions workflow builds all three platforms.

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

**Editing.** Edit the markdown of the page you are reading and save it back: local files first (the source view becomes an
editor with a live preview, and saving writes the file). Saving a remote page needs a write protocol the server accepts (HTTP `PUT`
with the saved credentials, WebDAV, or a Git host's API), so that comes after, one protocol at a time.

**Social media.** Not defined yet. Ideas to choose from: share a page, a selection or an AI summary as a post (copy as formatted text, or
post to Mastodon or Bluesky with an app password kept in the keychain), and save links from a feed or a timeline as bookmarks.

**AI features**, all through the same provider (see **AI assistant** above), in this order:

1. **More organizing.** Tidy an overgrown folder into subfolders, or propose a folder structure for a messy collection (sorting the unfiled is done).
2. **Reading tools.** Translate a page, explain or simplify a selection, an outline of long pages, a digest of an RSS feed.
3. **Compare tabs.** Summarize several open tabs together.
4. **Save as notes.** Turn the current page into a local markdown summary with a link back.
5. **Native Claude backend.** The Messages API for Claude-only features such as prompt caching, behind the same provider interface.

## Layout

```
main.go, app.go        Wails bootstrap and the methods exposed to the UI
internal/fetch         HTTP/file loading, content negotiation, size and redirect limits
internal/chats         saved Ask this page conversations
internal/ai            OpenAI-compatible chat client; summary and bookmark-suggestion prompts
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

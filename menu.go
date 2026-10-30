package main

import (
	"context"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	goruntime "runtime"
)

var onMac = goruntime.GOOS == "darwin"

// hint is how a shortcut reads in a menu on Windows and Linux: Ctrl+Shift+C.
func hint(a *keys.Accelerator) string {
	if a == nil {
		return ""
	}
	var parts []string
	for _, m := range a.Modifiers {
		switch m {
		case keys.CmdOrCtrlKey:
			parts = append(parts, "Ctrl")
		case keys.ShiftKey:
			parts = append(parts, "Shift")
		}
	}
	return strings.Join(append(parts, strings.ToUpper(a.Key)), "+")
}

// label is a menu item's text with its shortcut. On Windows and Linux the page handles the keys
// (the same ones as before the menu existed), so the menu only shows them; a tab sets it off to the right.
func label(text string, accel *keys.Accelerator) string {
	if onMac || accel == nil {
		return text
	}
	return text + "\t" + hint(accel)
}

// appMenu is the menu bar. Each item sends its id to the page as a "menu" event, and the page runs it
// (see menuActions in main.js). On macOS the menu owns the shortcuts and the page skips them; on Windows and
// Linux the page keeps handling the keys and the menu just lists them.
func (a *App) appMenu() *menu.Menu {
	send := func(id string) menu.Callback {
		// A goroutine: the page cannot be reached from inside a menu's own callback, which runs on the UI thread.
		return func(*menu.CallbackData) { go runtime.EventsEmit(a.ctx, "menu", id) }
	}
	item := func(m *menu.Menu, text, id string, accel *keys.Accelerator) {
		if !onMac {
			m.AddText(label(text, accel), nil, send(id))
			return
		}
		m.AddText(text, accel, send(id))
	}
	cmd := keys.CmdOrCtrl
	cmdShift := func(k string) *keys.Accelerator { return keys.Combo(k, keys.CmdOrCtrlKey, keys.ShiftKey) }

	bar := menu.NewMenu()
	if onMac {
		bar.Append(menu.AppMenu())
	}

	file := bar.AddSubmenu("File")
	item(file, "New Tab", "new-tab", cmd("t"))
	item(file, "Close Tab", "close-tab", cmd("w"))
	file.AddSeparator()
	item(file, "Edit Page", "edit-page", cmd("e"))
	item(file, "Save Page…", "save-page", cmd("s"))
	item(file, "Copy Markdown", "copy-markdown", cmdShift("c"))
	if !onMac {
		file.AddSeparator()
		file.AddText("Exit", nil, func(*menu.CallbackData) { runtime.Quit(a.ctx) })
	}

	if onMac {
		bar.Append(menu.EditMenu())
	} else {
		// Cut, copy and paste work in the page's own fields; Select All depends on where you are.
		edit := bar.AddSubmenu("Edit")
		item(edit, "Select All", "select-all", cmd("a"))
	}

	view := bar.AddSubmenu("View")
	item(view, "Reload", "reload", cmd("r"))
	if onMac {
		item(view, "Back", "back", cmd("["))
		item(view, "Forward", "forward", cmd("]"))
	} else {
		view.AddText("Back\tAlt+Left", nil, send("back"))
		view.AddText("Forward\tAlt+Right", nil, send("forward"))
	}
	view.AddSeparator()
	item(view, "Focus Address Bar", "address", cmd("l"))
	view.AddSeparator()
	// These four say Show or Hide, kept up to date by SetViewShown.
	a.viewItems = map[string]*menu.MenuItem{}
	toggle := func(id, name string, accel *keys.Accelerator) {
		viewKeys[id] = accel
		if onMac {
			a.viewItems[id] = view.AddText("Show "+name, accel, send(id))
		} else {
			a.viewItems[id] = view.AddText(label("Show "+name, accel), nil, send(id))
		}
	}
	toggle("bookmarks", "Bookmarks", cmd("b"))
	toggle("console", "Console", cmd("j"))
	toggle("source", "Source", cmd("u"))
	toggle("ask", "Ask Panel", cmdShift("a"))

	marks := bar.AddSubmenu("Bookmarks")
	item(marks, "Bookmark This Page…", "bookmark-page", cmd("d"))
	item(marks, "New Folder…", "bookmarks-new-folder", nil)
	marks.AddSeparator()
	item(marks, "Sort Unfiled with AI…", "bookmarks-organize", nil)
	item(marks, "Tag with AI…", "bookmarks-tag", nil)
	marks.AddSeparator()
	item(marks, "Import…", "bookmarks-import", nil)
	item(marks, "Export as HTML…", "bookmarks-export", nil)
	item(marks, "Export as Markdown…", "bookmarks-export-md", nil)
	marks.AddSeparator()
	item(marks, "Bookmark Storage…", "bookmarks-storage", nil)

	tools := bar.AddSubmenu("Tools")
	item(tools, "Summarize Page", "summary", nil)
	item(tools, "AI Assistant…", "ai", nil)
	item(tools, "Site Credentials…", "credentials", nil)
	tools.AddSeparator()
	item(tools, "History", "history", nil)
	item(tools, "Clear History", "clear-history", nil)

	if onMac {
		bar.Append(menu.WindowMenu())
	}
	return bar
}

// domReady points Edit > Select All at the page. It waits for the page because Wails
// installs the menu bar when the window opens.
func (a *App) domReady(context.Context) {
	// A goroutine: Wails cannot talk to the page from inside the menu's own callback, which runs on the UI thread.
	onSelectAll = func() { go runtime.EventsEmit(a.ctx, "menu", "select-all") }
	routeSelectAll()
}

// viewKeys remembers each toggle's shortcut, for relabelling it.
var viewKeys = map[string]*keys.Accelerator{}

// viewNames is what each View menu toggle is called after "Show " / "Hide ".
var viewNames = map[string]string{"bookmarks": "Bookmarks", "console": "Console", "source": "Source", "ask": "Ask Panel"}

// SetViewShown keeps a View menu item saying what it will do: Show or Hide.
func (a *App) SetViewShown(id string, shown bool) {
	item := a.viewItems[id]
	if item == nil {
		return
	}
	verb := "Show "
	if shown {
		verb = "Hide "
	}
	text := label(verb+viewNames[id], viewKeys[id])
	if item.Label == text {
		return
	}
	item.SetLabel(text)
	runtime.MenuUpdateApplicationMenu(a.ctx)
	routeSelectAll() // the update builds the menu bar again
}

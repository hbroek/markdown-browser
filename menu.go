package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// macMenu is the menu bar on macOS. Each item sends its id to the page as a "menu" event,
// and the page runs it (see menuActions in main.js). The page does not handle those shortcuts itself on macOS.
func (a *App) macMenu() *menu.Menu {
	send := func(id string) menu.Callback {
		return func(*menu.CallbackData) { runtime.EventsEmit(a.ctx, "menu", id) }
	}
	item := func(m *menu.Menu, label, id string, accel *keys.Accelerator) {
		m.AddText(label, accel, send(id))
	}
	cmd := keys.CmdOrCtrl
	cmdShift := func(k string) *keys.Accelerator { return keys.Combo(k, keys.CmdOrCtrlKey, keys.ShiftKey) }

	bar := menu.NewMenu()
	bar.Append(menu.AppMenu())

	file := bar.AddSubmenu("File")
	item(file, "New Tab", "new-tab", cmd("t"))
	item(file, "Close Tab", "close-tab", cmd("w"))
	file.AddSeparator()
	item(file, "Save Page…", "save-page", cmd("s"))
	item(file, "Copy Markdown", "copy-markdown", cmdShift("c"))

	bar.Append(menu.EditMenu())

	view := bar.AddSubmenu("View")
	item(view, "Reload", "reload", cmd("r"))
	item(view, "Back", "back", cmd("["))
	item(view, "Forward", "forward", cmd("]"))
	view.AddSeparator()
	item(view, "Focus Address Bar", "address", cmd("l"))
	view.AddSeparator()
	// These four say Show or Hide, kept up to date by SetViewShown.
	a.viewItems = map[string]*menu.MenuItem{}
	toggle := func(id, name string, accel *keys.Accelerator) {
		a.viewItems[id] = view.AddText("Show "+name, accel, send(id))
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

	bar.Append(menu.WindowMenu())
	return bar
}

// domReady points Edit > Select All at the page. It waits for the page because Wails
// installs the menu bar when the window opens.
func (a *App) domReady(context.Context) {
	// A goroutine: Wails cannot talk to the page from inside the menu's own callback, which runs on the UI thread.
	onSelectAll = func() { go runtime.EventsEmit(a.ctx, "menu", "select-all") }
	routeSelectAll()
}

// viewNames is what each View menu toggle is called after "Show " / "Hide ".
var viewNames = map[string]string{"bookmarks": "Bookmarks", "console": "Console", "source": "Source", "ask": "Ask Panel"}

// SetViewShown keeps a View menu item saying what it will do: Show or Hide.
func (a *App) SetViewShown(id string, shown bool) {
	item := a.viewItems[id]
	if item == nil {
		return
	}
	label := "Show " + viewNames[id]
	if shown {
		label = "Hide " + viewNames[id]
	}
	if item.Label == label {
		return
	}
	item.SetLabel(label)
	runtime.MenuUpdateApplicationMenu(a.ctx)
	routeSelectAll() // the update builds the menu bar again
}

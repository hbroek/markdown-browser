package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void disableWindowTabbing(void);
void routeSelectAll(void);
*/
import "C"

// macOS adds "Show Tab Bar" and "Merge All Windows" for its own window tabs. The browser has
// its own tabs, and the system's bar only shows the window title, so switch them off.
func init() { C.disableWindowTabbing() }

// onSelectAll runs when Edit > Select All is chosen; main sets it.
var onSelectAll func()

//export goSelectAll
func goSelectAll() {
	if onSelectAll != nil {
		onSelectAll()
	}
}

// routeSelectAll sends Edit > Select All to onSelectAll instead of the web view.
func routeSelectAll() { C.routeSelectAll() }

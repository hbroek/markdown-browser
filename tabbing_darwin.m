#import <Cocoa/Cocoa.h>

extern void goSelectAll(void);

// Called from Go: see tabbing_darwin.go.
void disableWindowTabbing(void) {
    [NSWindow setAllowsAutomaticWindowTabbing:NO];
}

@interface SelectAllTarget : NSObject
- (void)selectAllClicked:(id)sender;
@end

@implementation SelectAllTarget
- (void)selectAllClicked:(id)sender {
    goSelectAll();
}
@end

// Edit > Select All selects the whole web view by default. Send it to the page instead, which
// knows whether the bookmarks, the page or the chat is meant.
void routeSelectAll(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        static SelectAllTarget *target;
        if (!target) target = [SelectAllTarget new];
        NSMenu *edit = [[[NSApp mainMenu] itemWithTitle:@"Edit"] submenu];
        for (NSMenuItem *item in edit.itemArray) {
            if (item.action == @selector(selectAll:)) {
                item.target = target;
                item.action = @selector(selectAllClicked:);
            }
        }
    });
}

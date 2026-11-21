//go:build darwin && cgo

#import <AppKit/AppKit.h>
#include <stdint.h>

void canveppPrepareAssistantWindow(uintptr_t handle) {
    NSWindow *window = (NSWindow *)handle;
    // Keep cross-Space placement while using an activating, keyboard-ready window.
    window.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces
        | NSWindowCollectionBehaviorFullScreenAuxiliary
        | NSWindowCollectionBehaviorIgnoresCycle
        | NSWindowCollectionBehaviorTransient;
    for (NSView *view in window.contentView.subviews) {
        if ([view conformsToProtocol:@protocol(NSTextInputClient)]) {
            window.initialFirstResponder = view;
            [window makeFirstResponder:view];
            break;
        }
    }
}

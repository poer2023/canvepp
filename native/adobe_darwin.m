#import <AppKit/AppKit.h>
int canveppActiveDesignHost(void) {
    @autoreleasepool {
        NSString *host = [[[NSWorkspace sharedWorkspace] frontmostApplication] bundleIdentifier].lowercaseString;
        if ([host isEqualToString:@"com.adobe.photoshop"]) return 1;
        if ([host isEqualToString:@"com.adobe.illustrator"]) return 2;
        return 0;
    }
}

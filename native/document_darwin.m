#import <Foundation/Foundation.h>
#import <PDFKit/PDFKit.h>
#include <stdlib.h>
#include <string.h>

char *canveppPDFText(const char *path, int limit, int *status) {
    @autoreleasepool {
        *status = 1;
        @try {
            PDFDocument *doc = [[PDFDocument alloc] initWithURL:
                [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]]];
            if (!doc) return NULL;
            if (doc.isLocked || !doc.allowsCopying) {
                *status = 2; [doc release]; return NULL;
            }
            if (doc.pageCount > 200) {
                *status = 3; [doc release]; return NULL;
            }
            NSMutableString *text = [NSMutableString string];
            for (NSUInteger page = 0; page < doc.pageCount; page++) {
                @autoreleasepool {
                    NSString *part = [doc pageAtIndex:page].string;
                    if (part.length > 0) {
                        [text appendFormat:@"\n[第 %lu 页]\n%@\n", (unsigned long)page + 1, part];
                    }
                    if ([text lengthOfBytesUsingEncoding:NSUTF8StringEncoding] > (NSUInteger)limit) {
                        *status = 3; [doc release]; return NULL;
                    }
                }
            }
            [doc release];
            if (text.length == 0) { *status = 4; return NULL; }
            *status = 0;
            return strdup(text.UTF8String);
        } @catch (NSException *exception) {
            return NULL;
        }
    }
}

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

void canveppGenerationNotice(const char *id, const char *title, const char *body, const char *image) {
    @autoreleasepool {
        NSString *identifier = [NSString stringWithUTF8String:id];
        NSString *heading = [NSString stringWithUTF8String:title];
        NSString *message = [NSString stringWithUTF8String:body];
        NSString *path = [NSString stringWithUTF8String:image];
        dispatch_async(dispatch_get_main_queue(), ^{
            UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
            [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert
                completionHandler:^(BOOL allowed, NSError *authorizationError) {
                    if (!allowed) {
                        if (path.length) [[NSFileManager defaultManager] removeItemAtPath:path error:NULL];
                        if (authorizationError) NSLog(@"Canvepp notification permission (%ld)", (long)authorizationError.code);
                        return;
                    }
                    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
                    content.title = heading;
                    content.body = message;
                    if (path.length) {
                        UNNotificationAttachment *attachment = [UNNotificationAttachment
                            attachmentWithIdentifier:@"generation-preview"
                            URL:[NSURL fileURLWithPath:path] options:nil error:NULL];
                        if (attachment) content.attachments = @[attachment];
                    }
                    UNNotificationRequest *request = [UNNotificationRequest
                        requestWithIdentifier:identifier content:content trigger:nil];
                    [center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
                        if (path.length) [[NSFileManager defaultManager] removeItemAtPath:path error:NULL];
                        if (error) NSLog(@"Canvepp notification delivery (%ld)", (long)error.code);
                    }];
                    [content release];
                }];
        });
    }
}

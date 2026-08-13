//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework Foundation -framework UserNotifications

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>

static void DJI4GConnectShowNotification(const char *titleCString, const char *messageCString) {
    @autoreleasepool {
        NSString *title = [NSString stringWithUTF8String:titleCString];
        NSString *message = [NSString stringWithUTF8String:messageCString];
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

        dispatch_semaphore_t authorizationSemaphore = dispatch_semaphore_create(0);
        __block BOOL notificationAllowed = NO;
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *error) {
            notificationAllowed = granted && error == nil;
            dispatch_semaphore_signal(authorizationSemaphore);
        }];
        dispatch_semaphore_wait(authorizationSemaphore,
                                dispatch_time(DISPATCH_TIME_NOW, 15 * NSEC_PER_SEC));
        if (!notificationAllowed) {
            return;
        }

        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = title;
        content.body = message;
        content.sound = [UNNotificationSound defaultSound];

        NSString *identifier = [NSString stringWithFormat:@"io.github.wongloki.dji4gconnect.%@",
                                [[NSUUID UUID] UUIDString]];
        UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier
                                                                                content:content
                                                                                trigger:nil];
        dispatch_semaphore_t deliverySemaphore = dispatch_semaphore_create(0);
        [center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
            dispatch_semaphore_signal(deliverySemaphore);
        }];
        dispatch_semaphore_wait(deliverySemaphore,
                                dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
    }
}
*/
import "C"

import "unsafe"

func showNotification(title, message string) {
	titleCString := C.CString(title)
	messageCString := C.CString(message)
	defer C.free(unsafe.Pointer(titleCString))
	defer C.free(unsafe.Pointer(messageCString))
	C.DJI4GConnectShowNotification(titleCString, messageCString)
}

//go:build darwin

package app

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>

static void setMacActivationPolicy(bool regular, const void *iconBytes, int iconLen) {
	void (^apply)(void) = ^{
		if (NSApp == nil) {
			return;
		}
		if (regular) {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
			static NSImage *cachedIcon = nil;
			if (cachedIcon == nil && iconBytes != NULL && iconLen > 0) {
				NSData *data = [[NSData alloc] initWithBytes:iconBytes length:(NSUInteger)iconLen];
				if (data != nil) {
					cachedIcon = [[NSImage alloc] initWithData:data];
					[data release];
				}
			}
			if (cachedIcon != nil) {
				[NSApp setApplicationIconImage:cachedIcon];
			}
			[NSApp unhide:nil];
			[NSApp activateIgnoringOtherApps:YES];
		} else {
			[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		}
	};
	if ([NSThread isMainThread]) {
		apply();
	} else {
		dispatch_sync(dispatch_get_main_queue(), apply);
	}
}
*/
import "C"
import "unsafe"

func setDockVisible(visible bool, appIcon []byte) {
	var ptr unsafe.Pointer
	var length C.int
	if len(appIcon) > 0 {
		ptr = unsafe.Pointer(&appIcon[0])
		length = C.int(len(appIcon))
	}
	C.setMacActivationPolicy(C.bool(visible), ptr, length)
}

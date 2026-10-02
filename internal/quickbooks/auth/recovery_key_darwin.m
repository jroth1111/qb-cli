//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>

OSStatus qbRecoveryLookup(const char *account, CFDataRef *data) {
    @autoreleasepool {
        LAContext *context = [[LAContext alloc] init];
        context.interactionNotAllowed = YES;
        NSDictionary *query = @{
            (id)kSecClass: (id)kSecClassGenericPassword,
            (id)kSecAttrService: @"qb login recovery",
            (id)kSecAttrAccount: [NSString stringWithUTF8String:account],
            (id)kSecReturnData: @YES,
            (id)kSecMatchLimit: (id)kSecMatchLimitOne,
            (id)kSecUseAuthenticationContext: context
        };
        OSStatus status = SecItemCopyMatching((CFDictionaryRef)query, (CFTypeRef *)data);
        [context release];
        return status;
    }
}

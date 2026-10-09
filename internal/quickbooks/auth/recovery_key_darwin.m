//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>

#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

OSStatus qbRecoveryLookup(const char *account, CFDataRef *data) {
    @autoreleasepool {
        // File-based login Keychains do not honor the SecItem UI controls alone.
        // Keep this process's interaction policy scoped to the serialized lookup.
        Boolean wasAllowed;
        OSStatus policyStatus = SecKeychainGetUserInteractionAllowed(&wasAllowed);
        if (policyStatus != errSecSuccess) return policyStatus;
        policyStatus = SecKeychainSetUserInteractionAllowed(false);
        if (policyStatus != errSecSuccess) return policyStatus;
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
        SecKeychainSetUserInteractionAllowed(wasAllowed);
        return status;
    }
}
#pragma clang diagnostic pop

#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>

#include <stdlib.h>
#include <string.h>

#include "keychain.h"

static NSString *const serviceName = @"com.theronburger.key-session";

static NSMutableDictionary *baseQuery(NSString *account) {
    return [@{
        (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: serviceName,
        (__bridge id)kSecAttrAccount: account,
        (__bridge id)kSecUseDataProtectionKeychain: @YES,
    } mutableCopy];
}

static LAContext *authenticationContext(NSString *reason) {
    LAContext *context = [[LAContext alloc] init];
    context.localizedReason = reason;
    return context;
}

static void setErrorMessage(char **errorMessage, OSStatus status) {
    if (errorMessage == NULL) {
        return;
    }
    CFStringRef statusMessage = SecCopyErrorMessageString(status, NULL);
    if (statusMessage == NULL) {
        *errorMessage = strdup("unknown Keychain error");
        return;
    }
    *errorMessage = strdup([(__bridge NSString *)statusMessage UTF8String]);
    CFRelease(statusMessage);
}

static void setTextError(char **errorMessage, NSString *message) {
    if (errorMessage != NULL) {
        *errorMessage = strdup([message UTF8String]);
    }
}

int key_session_keychain_store(const char *accountValue, const void *secret, size_t secretLength, char **errorMessage) {
    @autoreleasepool {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        if (account == nil || secret == NULL || secretLength == 0) {
            setTextError(errorMessage, @"invalid profile or secret");
            return 1;
        }

        CFErrorRef accessError = NULL;
        SecAccessControlRef accessControl = SecAccessControlCreateWithFlags(
            NULL,
            kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
            kSecAccessControlUserPresence,
            &accessError
        );
        if (accessControl == NULL) {
            NSString *message = accessError == NULL
                ? @"could not create user-presence protection"
                : [(__bridge NSError *)accessError localizedDescription];
            setTextError(errorMessage, message);
            if (accessError != NULL) {
                CFRelease(accessError);
            }
            return 1;
        }

        NSMutableDictionary *deleteQuery = baseQuery(account);
        LAContext *replaceContext = authenticationContext(@"Replace the saved key-session secret");
        deleteQuery[(__bridge id)kSecUseAuthenticationContext] = replaceContext;
        OSStatus deleteStatus = SecItemDelete((__bridge CFDictionaryRef)deleteQuery);
        [replaceContext release];
        [deleteQuery release];
        if (deleteStatus != errSecSuccess && deleteStatus != errSecItemNotFound) {
            CFRelease(accessControl);
            setErrorMessage(errorMessage, deleteStatus);
            return 1;
        }

        NSMutableDictionary *addQuery = baseQuery(account);
        addQuery[(__bridge id)kSecAttrAccessControl] = (__bridge id)accessControl;
        addQuery[(__bridge id)kSecValueData] = [NSData dataWithBytes:secret length:secretLength];
        OSStatus addStatus = SecItemAdd((__bridge CFDictionaryRef)addQuery, NULL);
        [addQuery release];
        CFRelease(accessControl);
        if (addStatus != errSecSuccess) {
            setErrorMessage(errorMessage, addStatus);
            return 1;
        }
        return 0;
    }
}

int key_session_keychain_read(const char *accountValue, void **secret, size_t *secretLength, char **errorMessage) {
    @autoreleasepool {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        if (account == nil || secret == NULL || secretLength == NULL) {
            setTextError(errorMessage, @"invalid profile");
            return 1;
        }

        NSMutableDictionary *query = baseQuery(account);
        query[(__bridge id)kSecReturnData] = @YES;
        query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
        LAContext *context = authenticationContext(@"Start the requested key-session lease");
        query[(__bridge id)kSecUseAuthenticationContext] = context;

        CFTypeRef result = NULL;
        OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
        [context release];
        [query release];
        if (status != errSecSuccess) {
            setErrorMessage(errorMessage, status);
            return 1;
        }

        NSData *data = (__bridge NSData *)result;
        *secretLength = [data length];
        *secret = malloc(*secretLength);
        if (*secret == NULL) {
            CFRelease(result);
            setTextError(errorMessage, @"could not allocate secret buffer");
            return 1;
        }
        memcpy(*secret, [data bytes], *secretLength);
        CFRelease(result);
        return 0;
    }
}

int key_session_keychain_delete(const char *accountValue, char **errorMessage) {
    @autoreleasepool {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        if (account == nil) {
            setTextError(errorMessage, @"invalid profile");
            return 1;
        }
        NSMutableDictionary *query = baseQuery(account);
        LAContext *context = authenticationContext(@"Delete the saved key-session secret");
        query[(__bridge id)kSecUseAuthenticationContext] = context;
        OSStatus status = SecItemDelete((__bridge CFDictionaryRef)query);
        [context release];
        [query release];
        if (status != errSecSuccess && status != errSecItemNotFound) {
            setErrorMessage(errorMessage, status);
            return 1;
        }
        return 0;
    }
}

void key_session_keychain_free(void *pointer) {
    free(pointer);
}

#import <Foundation/Foundation.h>
#import <Security/Security.h>
#import <Security/SecAccess.h>
#import <Security/SecACL.h>

#include <stdlib.h>
#include <string.h>

#include "keychain.h"

static NSString *const serviceName = @"com.theronburger.key-session";

static NSMutableDictionary *baseQuery(NSString *account) {
    return [@{
        (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: serviceName,
        (__bridge id)kSecAttrAccount: account,
    } mutableCopy];
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

static SecAccessRef createPromptingAccess(NSString *account, char **errorMessage) {
    // Data-protection Keychain access requires a provisioned app wrapper; a standalone CLI must use the login Keychain ACL API.
    NSString *description = [NSString stringWithFormat:@"key-session profile %@", account];
    CFArrayRef noTrustedApplications = CFArrayCreate(
        NULL,
        NULL,
        0,
        &kCFTypeArrayCallBacks
    );
    if (noTrustedApplications == NULL) {
        setTextError(errorMessage, @"could not create an empty Keychain trust list");
        return NULL;
    }

    SecAccessRef access = NULL;
    OSStatus accessStatus = SecAccessCreate(
        (__bridge CFStringRef)description,
        noTrustedApplications,
        &access
    );
    if (accessStatus != errSecSuccess) {
        CFRelease(noTrustedApplications);
        setErrorMessage(errorMessage, accessStatus);
        return NULL;
    }

    CFArrayRef restrictedACLs = SecAccessCopyMatchingACLList(access, kSecACLAuthorizationDecrypt);
    if (restrictedACLs == NULL || CFArrayGetCount(restrictedACLs) == 0) {
        if (restrictedACLs != NULL) {
            CFRelease(restrictedACLs);
        }
        CFRelease(noTrustedApplications);
        CFRelease(access);
        setTextError(errorMessage, @"could not locate the Keychain password access rule");
        return NULL;
    }

    for (CFIndex index = 0; index < CFArrayGetCount(restrictedACLs); index++) {
        SecACLRef acl = (SecACLRef)CFArrayGetValueAtIndex(restrictedACLs, index);
        OSStatus aclStatus = SecACLSetContents(
            acl,
            noTrustedApplications,
            (__bridge CFStringRef)description,
            kSecKeychainPromptRequirePassphase
        );
        if (aclStatus != errSecSuccess) {
            CFRelease(restrictedACLs);
            CFRelease(noTrustedApplications);
            CFRelease(access);
            setErrorMessage(errorMessage, aclStatus);
            return NULL;
        }
    }

    CFRelease(restrictedACLs);
    CFRelease(noTrustedApplications);
    return access;
}

int key_session_keychain_store(const char *accountValue, const void *secret, size_t secretLength, char **errorMessage) {
    @autoreleasepool {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        if (account == nil || secret == NULL || secretLength == 0) {
            setTextError(errorMessage, @"invalid profile or secret");
            return 1;
        }

        SecAccessRef access = createPromptingAccess(account, errorMessage);
        if (access == NULL) {
            return 1;
        }

        NSMutableDictionary *addQuery = baseQuery(account);
        addQuery[(__bridge id)kSecAttrAccess] = (__bridge id)access;
        addQuery[(__bridge id)kSecValueData] = [NSData dataWithBytes:secret length:secretLength];
        OSStatus addStatus = SecItemAdd((__bridge CFDictionaryRef)addQuery, NULL);
        [addQuery release];
        CFRelease(access);
        if (addStatus == errSecDuplicateItem) {
            NSMutableDictionary *query = baseQuery(account);
            NSDictionary *changes = @{
                (__bridge id)kSecValueData: [NSData dataWithBytes:secret length:secretLength],
            };
            OSStatus updateStatus = SecItemUpdate(
                (__bridge CFDictionaryRef)query,
                (__bridge CFDictionaryRef)changes
            );
            [query release];
            if (updateStatus != errSecSuccess) {
                setErrorMessage(errorMessage, updateStatus);
                return 1;
            }
            return 0;
        }
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

        CFTypeRef result = NULL;
        OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
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
        OSStatus status = SecItemDelete((__bridge CFDictionaryRef)query);
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

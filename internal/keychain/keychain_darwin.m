#import <Foundation/Foundation.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <Security/Security.h>
#import <Security/SecAccess.h>
#import <Security/SecTrustedApplication.h>

#include <stdlib.h>
#include <string.h>

#include "keychain.h"

// The macOS file-based Keychain ACL trusts the signed Key Session executables;
// the daemon still completes an explicit Touch ID gate before every read.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static NSString *const serviceName = @"com.theronburger.key-session.protected";

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

static int authenticateUserPresence(NSString *account, NSString *reason, char **errorMessage) {
    LAContext *context = [[LAContext alloc] init];
    context.localizedFallbackTitle = @"";
    NSError *policyError = nil;
    if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&policyError]) {
        setTextError(errorMessage, policyError.localizedDescription ?: @"Touch ID is unavailable or not enrolled");
        [context release];
        return 1;
    }

    dispatch_semaphore_t completion = dispatch_semaphore_create(0);
    __block BOOL approved = NO;
    __block NSString *failureMessage = nil;
    NSString *localizedReason = reason.length > 0
        ? reason
        : [NSString stringWithFormat:@"Reveal the Key Session profile %@.", account];
    [context evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics localizedReason:localizedReason reply:^(BOOL success, NSError *authenticationError) {
        approved = success;
        if (authenticationError != nil) {
            failureMessage = [authenticationError.localizedDescription copy];
        }
        dispatch_semaphore_signal(completion);
    }];
    dispatch_semaphore_wait(completion, DISPATCH_TIME_FOREVER);
    [context invalidate];
    [context release];

    if (!approved) {
        setTextError(errorMessage, failureMessage ?: @"Touch ID authentication was denied");
        [failureMessage release];
        return 1;
    }
    [failureMessage release];
    return 0;
}

static SecAccessRef createTrustedAccess(NSString *account, char **errorMessage) {
    NSMutableArray *trustedApplications = [NSMutableArray array];
    SecTrustedApplicationRef currentApplication = NULL;
    OSStatus currentStatus = SecTrustedApplicationCreateFromPath(NULL, &currentApplication);
    if (currentStatus != errSecSuccess || currentApplication == NULL) {
        setErrorMessage(errorMessage, currentStatus);
        return NULL;
    }
    [trustedApplications addObject:(__bridge id)currentApplication];
    CFRelease(currentApplication);

    NSString *installedHelper = [NSHomeDirectory() stringByAppendingPathComponent:
        @"Library/Application Support/key-session/Key Session Helper.app/Contents/MacOS/KeySessionDaemon"];
    if ([[NSFileManager defaultManager] isExecutableFileAtPath:installedHelper]) {
        SecTrustedApplicationRef helperApplication = NULL;
        OSStatus helperStatus = SecTrustedApplicationCreateFromPath(
            [installedHelper fileSystemRepresentation],
            &helperApplication
        );
        if (helperStatus == errSecSuccess && helperApplication != NULL) {
            [trustedApplications addObject:(__bridge id)helperApplication];
            CFRelease(helperApplication);
        }
    }

    SecAccessRef access = NULL;
    NSString *description = [NSString stringWithFormat:@"Key Session profile %@", account];
    OSStatus accessStatus = SecAccessCreate(
        (__bridge CFStringRef)description,
        (__bridge CFArrayRef)trustedApplications,
        &access
    );
    if (accessStatus != errSecSuccess) {
        setErrorMessage(errorMessage, accessStatus);
        return NULL;
    }
    return access;
}

static OSStatus storeProtectedSecret(NSString *account, NSData *data, char **errorMessage) {
    SecAccessRef access = createTrustedAccess(account, errorMessage);
    if (access == NULL) {
        return errSecParam;
    }

    NSMutableDictionary *addQuery = baseQuery(account);
    addQuery[(__bridge id)kSecAttrAccess] = (__bridge id)access;
    addQuery[(__bridge id)kSecValueData] = data;
    OSStatus status = SecItemAdd((__bridge CFDictionaryRef)addQuery, NULL);
    [addQuery release];

    if (status == errSecDuplicateItem) {
        NSMutableDictionary *deleteQuery = baseQuery(account);
        OSStatus deleteStatus = SecItemDelete((__bridge CFDictionaryRef)deleteQuery);
        [deleteQuery release];
        if (deleteStatus != errSecSuccess && deleteStatus != errSecItemNotFound) {
            CFRelease(access);
            setErrorMessage(errorMessage, deleteStatus);
            return deleteStatus;
        }

        addQuery = baseQuery(account);
        addQuery[(__bridge id)kSecAttrAccess] = (__bridge id)access;
        addQuery[(__bridge id)kSecValueData] = data;
        status = SecItemAdd((__bridge CFDictionaryRef)addQuery, NULL);
        [addQuery release];
    }
    CFRelease(access);
    if (status != errSecSuccess) {
        setErrorMessage(errorMessage, status);
    }
    return status;
}

int key_session_keychain_store(const char *accountValue, const void *secret, size_t secretLength, char **errorMessage) {
    @autoreleasepool {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        if (account == nil || secret == NULL || secretLength == 0) {
            setTextError(errorMessage, @"invalid profile or secret");
            return 1;
        }

        NSData *data = [NSData dataWithBytes:secret length:secretLength];
        if (storeProtectedSecret(account, data, errorMessage) != errSecSuccess) {
            return 1;
        }

        return 0;
    }
}

static int readKeychainSecret(const char *accountValue, const char *approvalMessageValue, void **secret, size_t *secretLength, char **errorMessage) {
        NSString *account = [NSString stringWithUTF8String:accountValue];
        NSString *approvalMessage = [NSString stringWithUTF8String:approvalMessageValue];
        if (account == nil || approvalMessage == nil || [approvalMessage length] == 0 || secret == NULL || secretLength == NULL) {
            setTextError(errorMessage, @"invalid profile or approval message");
            return 1;
        }

        if (authenticateUserPresence(account, approvalMessage, errorMessage) != 0) {
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

int key_session_keychain_read(const char *accountValue, const char *approvalMessageValue, void **secret, size_t *secretLength, char **errorMessage) {
    @autoreleasepool {
        return readKeychainSecret(accountValue, approvalMessageValue, secret, secretLength, errorMessage);
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

#pragma clang diagnostic pop

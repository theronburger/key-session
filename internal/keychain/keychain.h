#ifndef KEY_SESSION_KEYCHAIN_H
#define KEY_SESSION_KEYCHAIN_H

#include <stddef.h>

int key_session_keychain_store(const char *account, const void *secret, size_t secret_length, char **error_message);
int key_session_keychain_read(const char *account, void **secret, size_t *secret_length, char **error_message);
int key_session_keychain_delete(const char *account, char **error_message);
void key_session_keychain_free(void *pointer);

#endif

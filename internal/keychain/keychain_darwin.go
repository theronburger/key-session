package keychain

/*
#cgo LDFLAGS: -framework Foundation -framework Security -framework LocalAuthentication
#include <stdlib.h>
#include "keychain.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func Store(account string, secret []byte) error {
	if len(secret) == 0 {
		return fmt.Errorf("secret is empty")
	}
	accountCString := C.CString(account)
	defer C.free(unsafe.Pointer(accountCString))

	var errorMessage *C.char
	status := C.key_session_keychain_store(
		accountCString,
		unsafe.Pointer(&secret[0]),
		C.size_t(len(secret)),
		&errorMessage,
	)
	return bridgeError(status, errorMessage)
}

func Read(account string) ([]byte, error) {
	accountCString := C.CString(account)
	defer C.free(unsafe.Pointer(accountCString))

	var secretPointer unsafe.Pointer
	var secretLength C.size_t
	var errorMessage *C.char
	status := C.key_session_keychain_read(
		accountCString,
		&secretPointer,
		&secretLength,
		&errorMessage,
	)
	if err := bridgeError(status, errorMessage); err != nil {
		return nil, err
	}
	defer C.key_session_keychain_free(secretPointer)
	return C.GoBytes(secretPointer, C.int(secretLength)), nil
}

func Delete(account string) error {
	accountCString := C.CString(account)
	defer C.free(unsafe.Pointer(accountCString))

	var errorMessage *C.char
	status := C.key_session_keychain_delete(accountCString, &errorMessage)
	return bridgeError(status, errorMessage)
}

func bridgeError(status C.int, errorMessage *C.char) error {
	if errorMessage != nil {
		defer C.key_session_keychain_free(unsafe.Pointer(errorMessage))
	}
	if status == 0 {
		return nil
	}
	if errorMessage == nil {
		return fmt.Errorf("macOS Keychain operation failed")
	}
	return fmt.Errorf("macOS Keychain operation failed: %s", C.GoString(errorMessage))
}

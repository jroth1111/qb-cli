//go:build darwin && cgo

package auth

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation -framework Foundation -framework LocalAuthentication
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
OSStatus qbRecoveryLookup(const char *account, CFDataRef *data);
*/
import "C"

import (
	"context"
	"unsafe"

	"github.com/keybase/go-keychain"
)

func nativeRecoveryKey(ctx context.Context, id string, create []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if create != nil {
		item := keychain.NewGenericPassword("qb login recovery", id, "", create, "")
		item.SetSynchronizable(keychain.SynchronizableNo)
		item.SetAccessible(keychain.AccessibleWhenUnlocked)
		if err := keychain.AddItem(item); err != nil {
			return nil, ErrRecoveryKeyUnavailable
		}
		return create, nil
	}
	account := C.CString(id)
	defer C.free(unsafe.Pointer(account))
	var data C.CFDataRef
	if C.qbRecoveryLookup(account, &data) != 0 || data == 0 {
		return nil, ErrRecoveryKeyUnavailable
	}
	defer C.CFRelease(C.CFTypeRef(data))
	if C.CFDataGetLength(data) != 32 {
		return nil, ErrRecoveryKeyUnavailable
	}
	return C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), 32), nil
}

func deleteNativeRecoveryKey(_ context.Context, id string) error {
	err := keychain.DeleteGenericPasswordItem("qb login recovery", id)
	if err != nil && err != keychain.ErrorItemNotFound {
		return ErrRecoveryKeyUnavailable
	}
	return nil
}

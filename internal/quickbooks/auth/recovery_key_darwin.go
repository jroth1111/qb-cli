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
	"time"
	"unsafe"

	"github.com/keybase/go-keychain"
)

var nativeKeyGate = make(chan struct{}, 1)

func nativeRecoveryKey(ctx context.Context, id string, create []byte) ([]byte, error) {
	if create != nil {
		select {
		case nativeKeyGate <- struct{}{}:
		case <-ctx.Done():
			return nil, ErrRecoveryKeyUnavailable
		}
		defer func() { <-nativeKeyGate }()
		return nativeRecoveryKeyDirect(ctx, id, create)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return boundedNativeKey(ctx, nativeKeyGate, func() ([]byte, error) { return nativeRecoveryKeyDirect(ctx, id, nil) })
}

func nativeRecoveryKeyDirect(ctx context.Context, id string, create []byte) ([]byte, error) {
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

func deleteNativeRecoveryKey(ctx context.Context, id string) error {
	select {
	case nativeKeyGate <- struct{}{}:
	case <-ctx.Done():
		return ErrRecoveryKeyUnavailable
	}
	defer func() { <-nativeKeyGate }()
	err := keychain.DeleteGenericPasswordItem("qb login recovery", id)
	if err != nil && err != keychain.ErrorItemNotFound {
		return ErrRecoveryKeyUnavailable
	}
	return nil
}

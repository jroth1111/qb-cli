//go:build (!darwin && !linux) || (darwin && !cgo)

package auth

import "context"

func nativeRecoveryKey(context.Context, string, []byte) ([]byte, error) {
	return nil, ErrRecoveryKeyUnavailable
}
func deleteNativeRecoveryKey(context.Context, string) error { return ErrRecoveryKeyUnavailable }

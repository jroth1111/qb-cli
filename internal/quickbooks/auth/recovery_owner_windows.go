//go:build windows

package auth

import "os"

func recoveryFileOwned(os.FileInfo) bool { return false }

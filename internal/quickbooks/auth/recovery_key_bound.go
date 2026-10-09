package auth

import "context"

// Some native secret services can block inside a system call despite a cancelled
// Go context. Only one such call may remain outstanding; callers still return
// on their deadline and any late key bytes are discarded.
func boundedNativeKey(ctx context.Context, gate chan struct{}, operation func() ([]byte, error)) ([]byte, error) {
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ErrRecoveryKeyUnavailable
	}
	type result struct {
		key []byte
		err error
	}
	done := make(chan result)
	go func() {
		defer func() { <-gate }()
		key, err := operation()
		select {
		case done <- result{key, err}:
		case <-ctx.Done():
			clear(key)
		}
	}()
	select {
	case value := <-done:
		return value.key, value.err
	case <-ctx.Done():
		return nil, ErrRecoveryKeyUnavailable
	}
}

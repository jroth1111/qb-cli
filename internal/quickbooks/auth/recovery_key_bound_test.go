package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBlockedNativeKeyReturnsOnDeadlineAndDoesNotAccumulateCalls(t *testing.T) {
	gate := make(chan struct{}, 1)
	release := make(chan struct{})
	started := make(chan struct{})
	key := []byte("fixture-private-key")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	before := time.Now()
	_, err := boundedNativeKey(ctx, gate, func() ([]byte, error) { close(started); <-release; return key, nil })
	if !errors.Is(err, ErrRecoveryKeyUnavailable) || time.Since(before) > time.Second {
		t.Fatal("native provider ignored deadline")
	}
	<-started
	second, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if _, err := boundedNativeKey(second, gate, func() ([]byte, error) { t.Error("spawned another blocked native call"); return nil, nil }); !errors.Is(err, ErrRecoveryKeyUnavailable) {
		t.Fatal("busy native provider did not fail boundedly")
	}
	close(release)
	select {
	case gate <- struct{}{}:
		<-gate
	case <-time.After(time.Second):
		t.Fatal("native gate did not release after completion")
	}
	for _, value := range key {
		if value != 0 {
			t.Fatal("late key bytes were retained")
		}
	}
}

func TestNativeKeySuccessfulResultIsPreserved(t *testing.T) {
	key, err := boundedNativeKey(context.Background(), make(chan struct{}, 1), func() ([]byte, error) { return []byte("fixture"), nil })
	if err != nil || string(key) != "fixture" {
		t.Fatal("native key result lost")
	}
}

package auth

import (
	"context"
	"testing"
	"time"
)

func TestAllowManagedRemintGating(t *testing.T) {
	t.Setenv("QB_NO_MANAGED", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if allowManagedRemint(ctx) {
		t.Error("kill-switch must refuse managed remint")
	}
}

func TestAllowManagedRemintNeedsTime(t *testing.T) {
	t.Setenv("QB_NO_MANAGED", "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if allowManagedRemint(ctx) {
		t.Error("10s deadline cannot fit a warm refresh")
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel2()
	if !allowManagedRemint(ctx2) {
		t.Error("5m deadline should allow managed remint")
	}
}

func TestRemintManagedKillSwitch(t *testing.T) {
	t.Setenv("QB_NO_MANAGED", "1")
	if err := RemintManaged(context.Background()); err == nil {
		t.Error("kill-switch must fail RemintManaged without launching Chrome")
	}
}

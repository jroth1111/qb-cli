package client

import "context"

type MaintenancePhase string

const (
	MaintenanceBankingLive       MaintenancePhase = "banking_live"
	MaintenanceBrowserSignedOut  MaintenancePhase = "browser_signed_out"
	MaintenanceRecovering        MaintenancePhase = "recovering"
	MaintenanceBankingReverified MaintenancePhase = "banking_reverified"
	MaintenanceNativeVerified    MaintenancePhase = "native_verified"
)

// Only fixed phases and HTTP outcomes leave the maintenance path. Credentials,
// page text and provider errors never enter this diagnostic contract.
type MaintenanceEvent struct {
	Phase      MaintenancePhase `json:"phase"`
	HTTPStatus int              `json:"http_status,omitempty"`
}

type maintenanceObserverKey struct{}

func WithMaintenanceObserver(ctx context.Context, observer func(MaintenanceEvent)) context.Context {
	return context.WithValue(ctx, maintenanceObserverKey{}, observer)
}

func observeMaintenance(ctx context.Context, phase MaintenancePhase, status int) {
	if observer, ok := ctx.Value(maintenanceObserverKey{}).(func(MaintenanceEvent)); ok && observer != nil {
		observer(MaintenanceEvent{Phase: phase, HTTPStatus: status})
	}
}

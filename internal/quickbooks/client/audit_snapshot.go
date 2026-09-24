package client

import "encoding/json"

// Preserve the server's numeric literals instead of round-tripping historical
// transaction evidence through the summary projection's float64 values.
func (e *rawAuditEvent) UnmarshalJSON(data []byte) error {
	type plain rawAuditEvent
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var wire struct {
		What struct {
			Entity struct {
				Transaction json.RawMessage `json:"TRANSACTION"`
			} `json:"entity"`
		} `json:"what"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*e = rawAuditEvent(decoded)
	e.TransactionSnapshot = wire.What.Entity.Transaction
	return nil
}

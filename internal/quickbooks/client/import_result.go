package client

import (
	"encoding/json"
	"fmt"
)

func validateCSVImportResponse(raw []byte, expected int) error {
	var result struct {
		Success     *int `json:"noOfSuccess"`
		Duplicates  int  `json:"duplicates"`
		Overwrites  int  `json:"noOfOverwriteSuccess"`
		OtherErrors struct {
			Rows []json.RawMessage `json:"rows"`
		} `json:"otherErrors"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Success == nil {
		return fmt.Errorf("CSV import returned no validated success count; inspect feed before retrying")
	}
	if *result.Success != expected || result.Duplicates != 0 || result.Overwrites != 0 || len(result.OtherErrors.Rows) != 0 {
		return fmt.Errorf("CSV import incomplete: success=%d expected=%d duplicates=%d overwrites=%d errors=%d; inspect feed before retrying", *result.Success, expected, result.Duplicates, result.Overwrites, len(result.OtherErrors.Rows))
	}
	return nil
}

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// QBO tags ride the tags service the qbo-tags-ui widget uses:
//   GET    https://tags.api.intuit.com/tags        (all; "tags/{typeId}" = by group)
//   GET    https://tags.api.intuit.com/types       (tag groups)
//   POST   https://tags.api.intuit.com/tags        {tagName, tagColor, typeId}
//   PUT    https://tags.api.intuit.com/tags/{id}   {tagName, tagColor, typeId}
//   DELETE https://tags.api.intuit.com/tags/{id}
// Auth is browser_auth with the qbo-tags-ui apikey plus intuit-company-id —
// both live in the uri_host_headers jar entry for this host.

const (
	tagsHost    = "tags.api.intuit.com"
	tagsBaseURL = "https://tags.api.intuit.com/"
)

// tagDoc is the service's tag row (raw keys observed in the widget client).
type tagDoc struct {
	ID       string `json:"id"`
	TagName  string `json:"tagName"`
	TagColor string `json:"tagColor"`
	TypeID   string `json:"typeId"`
	Deleted  bool   `json:"deleted"`
}

func (t tagDoc) item() QueryItem {
	it := QueryItem{ID: t.ID, Name: t.TagName, Type: "Tag"}
	if t.TypeID != "" {
		it.AccountID = t.TypeID // group id surfaces under accountId in rows
	}
	if t.TagColor != "" {
		it.Item = t.TagColor
	}
	return it
}

// tagServiceError lifts {message,errorCode} out of a tags-service error body.
func tagServiceError(raw []byte) string { return errorMessage(raw) }

// PlannedTagURL is the dry-run surface for the tags read path.
func PlannedTagURL() string { return tagsBaseURL + "tags" }

// replayTags backs ReplayQuery("Tag", ...): GET tags.api.intuit.com/tags[/{id}].
func replayTags(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if limit < 1 {
		limit = 20
	}
	id = strings.TrimSpace(id)
	if id != "" {
		// Single-tag read: GET tags/{id} answers one object.
		resp, err := ac.doURIHost(ctx, http.MethodGet, tagsBaseURL+"tags/"+id, tagsHost, nil)
		if err != nil {
			return nil, fmt.Errorf("tags read: %w", err)
		}
		raw, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, fmt.Errorf("tags read: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: tagServiceError(raw)}
		}
		var t tagDoc
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, fmt.Errorf("tags read: malformed response %.200s", raw)
		}
		items := []QueryItem{}
		if !t.Deleted {
			items = append(items, t.item())
		}
		return &QueryResult{Status: resp.StatusCode, Entity: "Tag",
			Counts: map[string]int{"matched": len(items)}, Items: items}, nil
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, tagsBaseURL+"tags", tagsHost, nil)
	if err != nil {
		return nil, fmt.Errorf("tags read: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("tags read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: tagServiceError(raw)}
	}
	var rows []tagDoc
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("tags read: malformed response %.200s", raw)
	}
	items := make([]QueryItem, 0, len(rows))
	for _, t := range rows {
		if t.Deleted {
			continue
		}
		items = append(items, t.item())
		if len(items) >= limit {
			break
		}
	}
	return &QueryResult{Status: resp.StatusCode, Entity: "Tag",
		Counts: map[string]int{"matched": len(items)}, Items: items}, nil
}

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func transientReadError(err error) bool {
	// net/url.Error implements net.Error even when its wrapped cause is a
	// definite authentication/control stop, not a transient network failure.
	if errors.Is(err, auth.ErrRemintNeedsLogin) || errors.Is(err, auth.ErrSessionChanged) || errors.Is(err, auth.ErrEgoUserControl) || errors.Is(err, auth.ErrRecoveryAttention) || errors.Is(err, auth.ErrRecoveryKeyUnavailable) {
		return false
	}
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errEgoReadTransient) || errors.As(err, &network)
}

// readReliably buffers JSON query responses before exposing them to a pager.
// Failed/truncated pages never become committed rows, and writes never retry.
func readReliably(req *http.Request, dial func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return dial(req)
	}
	var last error
	for attempt := range 3 {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		resp, err := dial(req)
		retry := err != nil && transientReadError(err)
		if err == nil && resp != nil {
			retry = resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
			if retry {
				last = fmt.Errorf("query transient HTTP %d", resp.StatusCode)
				_ = drainAndClose(resp)
			} else {
				if req.Method != http.MethodHead && resp.StatusCode == 200 && strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "json") {
					raw, readErr := readBoundedBody(resp.Body)
					_ = resp.Body.Close()
					trimmed := bytes.TrimSpace(raw)
					// Some native services mislabel XML, PDF or JavaScript literals
					// as JSON. Their domain parser owns that contract.
					jsonContainer := len(trimmed) == 0 || trimmed[0] == '{' || trimmed[0] == '['
					// getTxnData intentionally returns JavaScript constructors
					// inside a JSON-shaped object; only that native route opts out.
					jsLiteral := strings.HasSuffix(req.URL.Path, "/getTxnData")
					if readErr != nil && !transientReadError(readErr) {
						return nil, readErr
					}
					if readErr != nil || jsonContainer && !jsLiteral && !json.Valid(raw) {
						last = fmt.Errorf("query response incomplete or malformed: %w", io.ErrUnexpectedEOF)
						retry = true
					} else {
						resp.Body = io.NopCloser(bytes.NewReader(raw))
						resp.ContentLength = int64(len(raw))
						return resp, nil
					}
				} else {
					return resp, nil
				}
			}
		} else {
			last = err
		}
		if !retry {
			return resp, err
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 500 * time.Millisecond)
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}
	}
	return nil, fmt.Errorf("read retry budget exhausted; no partial page committed: %w", last)
}

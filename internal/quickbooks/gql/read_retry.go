package gql

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

var errBrowserQueryTransport = errors.New("browser query transport interrupted")
var nonQueryToken = regexp.MustCompile(`(?i)\b(mutation|subscription)\b`)

// Only known query-kind documents can replay. Ambiguous documents and writes
// execute once; retries never commit data to a pagination accumulator.
func executeReadReliably(ctx context.Context, req Request, execute func(context.Context, Request) (*Response, error)) (*Response, error) {
	doc := ""
	if req.Op != nil {
		doc = strings.TrimSpace(req.Op.Document)
	}
	read := req.Op != nil && req.Op.Kind == "query" && doc != "" && !nonQueryToken.MatchString(doc)
	return retryReadResponse(ctx, read, true, func() (*Response, error) { return execute(ctx, req) })
}

func retryReadResponse(ctx context.Context, read, jsonBody bool, execute func() (*Response, error)) (*Response, error) {
	if !read {
		return execute()
	}
	var last error
	for attempt := range 3 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, err := execute()
		if errors.Is(err, auth.ErrEgoUserControl) || errors.Is(err, auth.ErrSessionChanged) || ctx.Err() != nil {
			return resp, err
		}
		var network net.Error
		retry := errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, errBrowserQueryTransport) || errors.As(err, &network)
		if err == nil && resp != nil {
			retry = resp.Status == 408 || resp.Status == 429 || resp.Status == 502 || resp.Status == 503 || resp.Status == 504 || jsonBody && resp.Status == 200 && !json.Valid(resp.Body)
		}
		if !retry {
			return resp, err
		}
		last = err
		if last == nil {
			last = errors.New("query returned transient HTTP or incomplete JSON")
		}
		if attempt < 2 {
			timer := time.NewTimer(time.Duration(1<<attempt) * 500 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	return nil, last
}

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/enetx/http/httptrace"
	"github.com/enetx/surf"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// impersonatedTransport is an offline test seam; production leaves it nil.
var impersonatedTransport http.RoundTripper

type originalHeadersKey struct{}

type requestHeaderTransport struct{ base http.RoundTripper }

func (t requestHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Snapshot each hop after net/http has stripped credentials for a
	// cross-origin redirect. Never restore a previous hop's credentials.
	ctx := context.WithValue(req.Context(), originalHeadersKey{}, req.Header.Clone())
	request := req.Clone(ctx)
	switch request.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
	default:
		// surf otherwise replays a failed HTTP/2 exchange over HTTP/1.1,
		// including when the server already applied a mutation. Permit only
		// fallback before any request headers were sent (e.g. HTTP/1-only TLS).
		sent := new(atomic.Bool)
		request = request.WithContext(httptrace.WithClientTrace(request.Context(), &httptrace.ClientTrace{WroteHeaders: func() { sent.Store(true) }}))
		replay := request.GetBody
		bodyless := request.Body == nil || request.Body == http.NoBody
		if bodyless {
			request.Body = io.NopCloser(strings.NewReader(""))
		}
		request.GetBody = func() (io.ReadCloser, error) {
			if sent.Load() {
				return nil, fmt.Errorf("request may have been applied; refusing automatic replay")
			}
			if bodyless {
				return io.NopCloser(strings.NewReader("")), nil
			}
			if replay == nil {
				return nil, fmt.Errorf("request body cannot be replayed")
			}
			return replay()
		}
	}
	return t.base.RoundTrip(request)
}

func (t requestHeaderTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

var browserHTTPClient = sync.OnceValues(func() (*http.Client, error) {
	sc, err := surf.NewClient().Builder().Impersonate().Chrome().SecureTLS().Timeout(httpTimeout).
		With(func(req *surf.Request) error {
			// The browser profile supplies navigation Accept/Content-Type
			// defaults. Explicit API headers must win over those defaults.
			if headers, ok := req.GetRequest().Context().Value(originalHeadersKey{}).(http.Header); ok {
				for name, values := range headers {
					req.GetRequest().Header[name] = append([]string(nil), values...)
				}
			}
			// Chrome defaults include pseudo-header placeholders, but the
			// HTTP transport generates these from the URL and method.
			dropPseudoHeaders(req.GetRequest().Header)
			return nil
		}, 1000).Build().Result()
	if err != nil {
		return nil, err
	}
	client := sc.Std()
	client.Timeout = httpTimeout
	client.Jar = nil // Captured credentials, rather than an ambient jar, own cookies.
	client.Transport = requestHeaderTransport{base: client.Transport}
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		if previousRedirect != nil {
			if err := previousRedirect(req, via); err != nil {
				return err
			}
		}
		if len(via) > 0 && (req.URL.Scheme != via[0].URL.Scheme || !strings.EqualFold(req.URL.Host, via[0].URL.Host)) {
			// net/http only knows standard credential names. Captured API
			// keys, CSRF tokens and signed-URL referrers need the same boundary.
			for _, name := range []string{"Authorization", "Proxy-Authorization", "Cookie", "Apikey", "X-Api-Key", "Authtype", "Csrftoken", "X-Csrf-Token", "Intuit-Company-Id", "Intuit-User-Id", "Intuit_appid", "Referer"} {
				req.Header.Del(name)
			}
		}
		return nil
	}
	return client, nil
})

func dropPseudoHeaders(headers map[string][]string) {
	for name := range headers {
		if strings.HasPrefix(name, ":") {
			delete(headers, name)
		}
	}
}

// impersonatedDo is the sole HTTP request path for the QBO API client.
// Keep the complete request intact: verb, body, headers, context and redirects.
// Reuse the client so paginated requests can reuse connections. There is no
// plain-TLS fallback, including for downloads, uploads and error responses.
func sessionDo(req *http.Request, dial func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	var err error
	req, err = auth.PrepareSessionRequest(req)
	if err != nil {
		return nil, err
	}
	resp, err := readReliably(req, dial)
	if err == nil {
		if perr := auth.ObserveSessionResponse(req, resp); perr != nil && !errors.Is(perr, auth.ErrSessionChanged) {
			log.Print("qb: could not persist session rotation; request outcome is unchanged")
		}
	}
	return resp, err
}

func impersonatedDo(req *http.Request) (*http.Response, error) {
	return sessionDo(req, func(req *http.Request) (*http.Response, error) {
		if mode := os.Getenv("QB_HTTP_TRANSPORT"); mode != "" {
			if mode != "ego" {
				return nil, fmt.Errorf("unsupported QB_HTTP_TRANSPORT; use ego or leave unset")
			}
			return egoSessionDo(req)
		}
		if impersonatedTransport != nil {
			return (&http.Client{Transport: impersonatedTransport, Timeout: httpTimeout}).Do(req)
		}
		client, err := browserHTTPClient()
		if err != nil {
			return nil, err
		}
		return client.Do(req)
	})
}

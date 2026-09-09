// documents_test.go covers the document print/email replay surface in
// documents.go using the package-standard DefaultTransport swap
// (interceptHTTP): production qbo.intuit.com/document-host URLs are routed to
// a local httptest server, no production file is touched, and nothing here
// reaches the real network.
//
// Adequacy: the happy-path tests reject wrong method, wrong host path, wrong
// request body shape, and wrong response projection; the validation tests
// reject empty txn-id/email/ids before any client construction; the no-creds
// tests prove fast auth failure. Together they kill: (1) a handler that never
// sends, (2) one that sends to the wrong endpoint, (3) one that sends a
// malformed body, (4) one that accepts invalid input, and (5) one that dials
// without credentials.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// docServer captures requests per exact path so one server can serve every
// documents endpoint. Paths are registered as full "HOST PATH" patterns.
type docServer struct {
	t        *testing.T
	mu       chan struct{}
	lastPath string
	lastMeth string
	lastBody []byte

	status map[string]int
	bodies map[string]string
	ctype  map[string]string
}

func newDocServer(t *testing.T, status map[string]int, bodies map[string]string) *docServer {
	return &docServer{
		t:      t,
		mu:     make(chan struct{}, 1),
		status: status,
		bodies: bodies,
		ctype:  map[string]string{},
	}
}

func (s *docServer) handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(w http.ResponseWriter, r *http.Request) {
		s.mu <- struct{}{}
		s.lastPath = r.URL.Path
		s.lastMeth = r.Method
		b, _ := io_ReadAll(r.Body)
		_ = r.Body.Close()
		s.lastBody = b
		<-s.mu
		key := r.Method + " " + r.URL.Path
		if ct := s.ctype[key]; ct != "" {
			w.Header().Set("Content-Type", ct)
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(s.status[key])
		_, _ = w.Write([]byte(s.bodies[key]))
	}
	for _, p := range []string{
		"/v2/destinationEmail",
		"/v2/destinationEmail/checkAvailability",
		"/v2/destinationEmails",
		"/v2/print",
		"/v3/documents/pdf",
		"/v2/documents/list",
		"/v2/documents/177",
	} {
		mux.HandleFunc(p, handle)
	}
	return mux
}

func (s *docServer) start(t *testing.T) string {
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	return srv.URL
}

func (s *docServer) calls() (path, method string, body []byte) {
	return s.lastPath, s.lastMeth, s.lastBody
}

func TestReplayDocumentSendPostsDestinationAddress(t *testing.T) {
	saveUsable(t)
	fs := newDocServer(t,
		map[string]int{"POST /v2/destinationEmail": http.StatusOK},
		map[string]string{"POST /v2/destinationEmail": `{"email":"company@qbodocs.com"}`},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDestinationEmailSend(context.Background(), "177", "ops@example.com")
	if err != nil {
		t.Fatalf("ReplayDestinationEmailSend: %v", err)
	}
	if res.Status != http.StatusOK || res.Op != "send" || res.Email == "" {
		t.Fatalf("envelope = %+v", res)
	}
	path, method, body := fs.calls()
	if method != http.MethodPost || !strings.HasSuffix(path, "/v2/destinationEmail") || strings.HasSuffix(path, "/checkAvailability") {
		t.Fatalf("request = %s %s", method, path)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, body)
	}
	if got["destinationEmailAddress"] != "ops@example.com" {
		t.Fatalf("destinationEmailAddress = %v", got["destinationEmailAddress"])
	}
	if got["txnId"] != "177" {
		t.Fatalf("txnId = %v", got["txnId"])
	}
}

func TestReplayDestinationEmailGetReturnsCapturedShape(t *testing.T) {
	saveUsable(t)
	fs := newDocServer(t,
		map[string]int{"GET /v2/destinationEmail": http.StatusOK},
		map[string]string{"GET /v2/destinationEmail": `{"email":"company@qbodocs.com"}`},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDestinationEmailGet(context.Background())
	if err != nil {
		t.Fatalf("ReplayDestinationEmailGet: %v", err)
	}
	if res.Email != "company@qbodocs.com" {
		t.Fatalf("email = %q", res.Email)
	}
	path, method, _ := fs.calls()
	if method != http.MethodGet || !strings.HasSuffix(path, "/v2/destinationEmail") {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestReplayDestinationEmailCheckMapsStatuses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"available", http.StatusOK, true},
		{"invalid-400", http.StatusBadRequest, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			saveUsable(t)
			fs := newDocServer(t,
				map[string]int{"POST /v2/destinationEmail/checkAvailability": tc.status},
				map[string]string{"POST /v2/destinationEmail/checkAvailability": `{}`},
			)
			interceptHTTP(t, fs.start(t))
			res, err := ReplayDestinationEmailCheck(context.Background(), "custom@example.com.au")
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			if res.Available == nil || *res.Available != tc.want {
				t.Fatalf("available = %v, want %v", res.Available, tc.want)
			}
			_, _, body := fs.calls()
			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil || got["destinationEmailAddress"] == nil {
				t.Fatalf("body = %s (%v)", body, err)
			}
		})
	}
}

func TestReplayDestinationEmailsListProjectsHistory(t *testing.T) {
	saveUsable(t)
	fs := newDocServer(t,
		map[string]int{"GET /v2/destinationEmails": http.StatusOK},
		map[string]string{"GET /v2/destinationEmails": `{"destinationEmails":[{"id":"9","email":"old@qbodocs.com","createdAt":"2026-01-02"},{"id":"10","email":"new@qbodocs.com","createdAt":"2026-08-20"}]}`},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDestinationEmailsList(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(res.History) != 2 {
		t.Fatalf("history = %+v", res.History)
	}
	if res.Counts["items"] != 2 || res.History[1].Name != "new@qbodocs.com" {
		t.Fatalf("projection = %+v", res.History[1])
	}
}

func TestReplayDocumentsListPostsCapturedBodyShape(t *testing.T) {
	saveUsable(t)
	listBody := `{"documents":[` +
		`{"systemAttributes":{"id":"d1","createDate":"2026-08-01"},"commonAttributes":{"documentType":"receipt","documentName":"coles.png"}},` +
		`{"systemAttributes":{"id":"d2","createDate":"2026-08-09"},"commonAttributes":{"documentType":"statement","documentName":"aug.pdf"}}]}`
	fs := newDocServer(t,
		map[string]int{"POST /v2/documents/list": http.StatusOK},
		map[string]string{"POST /v2/documents/list": listBody},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDocumentsList(context.Background(), "document", []string{"d1", "d2"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 2 {
		t.Fatalf("envelope = %+v", res)
	}
	if res.Items[0].ID != "d1" || res.Items[0].Name != "coles.png" {
		t.Fatalf("item0 = %+v", res.Items[0])
	}
	_, _, body := fs.calls()
	var got struct {
		DocumentIDs     []string       `json:"documentIds"`
		IncludeEntities bool           `json:"includeEntityData"`
		ContextFilters  map[string]any `json:"contextFilters"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if len(got.DocumentIDs) != 2 || !got.IncludeEntities || got.ContextFilters["includeLocatorInfo"] != true {
		t.Fatalf("captured shape mismatch: %s", body)
	}
}

func TestReplayDocumentUpdatePutsJSONBody(t *testing.T) {
	saveUsable(t)
	fs := newDocServer(t,
		map[string]int{"PUT /v2/documents/177": http.StatusOK},
		map[string]string{"PUT /v2/documents/177": `{"systemAttributes":{"id":"177"},"commonAttributes":{"documentType":"receipt","documentName":"renamed.png"}}`},
	)
	interceptHTTP(t, fs.start(t))

	res, err := ReplayDocumentUpdate(context.Background(), "177", `{"commonAttributes":{"documentName":"renamed.png"}}`)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if res.Op != "update" || res.Entity != "Document" || res.Item.Name != "renamed.png" {
		t.Fatalf("envelope = %+v", res)
	}
	path, method, _ := fs.calls()
	if method != http.MethodPut || !strings.HasSuffix(path, "/v2/documents/177") {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestReplayDocumentUpdateRejectsNonObjectData(t *testing.T) {
	saveUsable(t)
	if _, err := ReplayDocumentUpdate(context.Background(), "177", `[1,2]`); err == nil {
		t.Fatal("array --data must be rejected")
	}
	if _, err := ReplayDocumentUpdate(context.Background(), "177", `not json`); err == nil {
		t.Fatal("non-JSON --data must be rejected")
	}
	if _, err := ReplayDocumentUpdate(context.Background(), "", `{}`); !errors.Is(err, ErrMissingMutateID) {
		t.Fatalf("empty id: got %v", err)
	}
}

func TestReplayDocumentPrintReportsPDFBytes(t *testing.T) {
	saveUsable(t)
	pdf := []byte("%PDF-1.7 fake payload bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/print" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Accept") != "application/pdf" {
			t.Errorf("Accept = %q, want application/pdf", r.Header.Get("Accept"))
		}
		b, _ := io_ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(b, &got); err != nil || got["txnId"] != "177" {
			t.Errorf("body = %s (%v)", b, err)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(pdf)
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	res, err := ReplayDocumentPrint(context.Background(), "177")
	if err != nil {
		t.Fatalf("print: %v", err)
	}
	if res.Status != http.StatusOK || res.Bytes != len(pdf) || res.ContentType != "application/pdf" {
		t.Fatalf("result = %+v", res)
	}
}

func TestReplayDocumentPDFWritesFile(t *testing.T) {
	saveUsable(t)
	pdf := "%PDF-1.7 downloaded"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v3/documents/pdf" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if q := r.URL.Query().Get("txnId"); q != "177" {
			t.Errorf("txnId = %q", q)
		}
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte(pdf))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)

	out := filepath.Join(t.TempDir(), "inv-177.pdf")
	res, err := ReplayDocumentPDF(context.Background(), "177", out)
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	if res.Bytes != len(pdf) || res.Path != out {
		t.Fatalf("result = %+v", res)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != pdf {
		t.Fatalf("file = %q (%v)", got, err)
	}
}

// --- API-error surfaces -----------------------------------------------------

func TestDocumentEndpointsSurfaceAPIErrors(t *testing.T) {
	saveUsable(t)
	fs := newDocServer(t,
		map[string]int{
			"POST /v2/destinationEmail": http.StatusBadRequest,
			"POST /v2/print":            http.StatusInternalServerError,
			"GET /v3/documents/pdf":     http.StatusNotFound,
			"POST /v2/documents/list":   http.StatusForbidden,
			"PUT /v2/documents/177":     http.StatusBadRequest,
			"GET /v2/destinationEmail":  http.StatusUnauthorized,
		},
		map[string]string{
			"POST /v2/destinationEmail": `{"error":{"message":"bad email"}}`,
			"POST /v2/print":            `{"error":{"message":"render failed"}}`,
			"GET /v3/documents/pdf":     `{"error":{"message":"no such txn"}}`,
			"POST /v2/documents/list":   `{"error":{"message":"denied"}}`,
			"PUT /v2/documents/177":     `{"error":{"message":"bad doc"}}`,
			"GET /v2/destinationEmail":  `{"error":{"message":"expired"}}`,
		},
	)
	interceptHTTP(t, fs.start(t))

	ctx := context.Background()
	if _, err := ReplayDestinationEmailSend(ctx, "177", "a@b.co"); err == nil {
		t.Error("send: expected error on 400")
	}
	if _, err := ReplayDestinationEmailGet(ctx); err == nil {
		t.Error("get: expected error on 401")
	}
	if _, err := ReplayDocumentPrint(ctx, "177"); err == nil {
		t.Error("print: expected error on 500")
	}
	if _, err := ReplayDocumentPDF(ctx, "177", filepath.Join(t.TempDir(), "x.pdf")); err == nil {
		t.Error("pdf: expected error on 404")
	}
	if _, err := ReplayDocumentsList(ctx, "document", []string{"d1"}); err == nil {
		t.Error("list: expected error on 403")
	}
	if _, err := ReplayDocumentUpdate(ctx, "177", "{}"); err == nil {
		t.Error("update: expected error on 400")
	}
}

// --- validation negatives ---------------------------------------------------

func TestDocumentValidationFailsBeforeClientConstruction(t *testing.T) {
	noCredsDir(t) // also proves these fire before credentials matter
	ctx := context.Background()

	if _, err := ReplayDestinationEmailSend(ctx, "", "a@b.co"); !errors.Is(err, ErrMissingTxnID) {
		t.Errorf("send without txn-id: got %v, want ErrMissingTxnID", err)
	}
	if _, err := ReplayDestinationEmailSend(ctx, "177", ""); !errors.Is(err, ErrMissingEmail) {
		t.Errorf("send without email: got %v, want ErrMissingEmail", err)
	}
	if _, err := ReplayDestinationEmailCheck(ctx, " "); !errors.Is(err, ErrMissingEmail) {
		t.Errorf("check with blank email: got %v", err)
	}
	if _, err := ReplayDocumentsList(ctx, "invoice", nil); !errors.Is(err, ErrMissingDocumentIDs) {
		t.Errorf("list without ids: got %v, want ErrMissingDocumentIDs", err)
	}
	if _, err := ReplayDocumentsList(ctx, "invoice", []string{" ", ""}); !errors.Is(err, ErrMissingDocumentIDs) {
		t.Errorf("list with only blank ids: got %v", err)
	}
	if _, err := ReplayDocumentPrint(ctx, ""); !errors.Is(err, ErrMissingTxnID) {
		t.Errorf("print without txn-id: got %v, want ErrMissingTxnID", err)
	}
	if _, err := ReplayDocumentPDF(ctx, "", "out.pdf"); !errors.Is(err, ErrMissingTxnID) {
		t.Errorf("pdf without txn-id: got %v, want ErrMissingTxnID", err)
	}
}

// --- no-credentials fast-fail ----------------------------------------------

func TestDocumentCallsFailFastOnNoCredentials(t *testing.T) {
	noCredsDir(t)
	ctx := context.Background()
	start := time.Now()

	_, err1 := ReplayDestinationEmailGet(ctx)
	_, err2 := ReplayDestinationEmailSend(ctx, "177", "a@b.co")
	_, err3 := ReplayDestinationEmailCheck(ctx, "a@b.co")
	_, err4 := ReplayDestinationEmailsList(ctx)
	_, err5 := ReplayDocumentsList(ctx, "document", []string{"d1"})
	_, err6 := ReplayDocumentUpdate(ctx, "177", "{}")
	_, err7 := ReplayDocumentPrint(ctx, "177")
	_, err8 := ReplayDocumentPDF(ctx, "177", filepath.Join(t.TempDir(), "x.pdf"))

	assertFast(t, time.Since(start))
	for i, err := range []error{err1, err2, err3, err4, err5, err6, err7, err8} {
		if err == nil {
			t.Fatalf("call %d: expected ErrNoCredentials, got nil", i)
		}
		if !errors.Is(err, ErrNoCredentials) {
			t.Errorf("call %d: got %v, want ErrNoCredentials", i, err)
		}
	}
}

// --- planned URLs ------------------------------------------------------------

func TestPlannedDocumentURLsUseRealmTokenAndHosts(t *testing.T) {
	for _, u := range []string{PlannedDocumentSendURL(), PlannedDocumentCheckURL(), PlannedDocumentEmailsURL(), PlannedDocumentPrintURL(), PlannedDocumentListURL(), PlannedDocumentPDFURL("17 7"), PlannedDocumentUpdateURL("177")} {
		if strings.Contains(u, "12345") { // session-realm fixture, not a pin
			t.Errorf("planned URL embeds live realm: %s", u)
		}
	}
	if u := PlannedDocumentPDFURL("17 7"); strings.Contains(u, " ") || strings.Contains(u, "%20") {
		t.Errorf("pdf plan did not sanitize id: %s", u)
	}
	if u := PlannedDocumentSendURL(); !strings.HasPrefix(u, "https://emailingestion.api.intuit.com/v2/destinationEmail") {
		t.Errorf("send plan = %s", u)
	}
	if u := PlannedDocumentPrintURL(); !strings.HasPrefix(u, "https://txnsrendering.api.intuit.com/v2/print") {
		t.Errorf("print plan = %s", u)
	}
	if u := PlannedDocumentListURL(); !strings.HasPrefix(u, "https://financialdocumentpci.api.intuit.com/v2/documents/list") {
		t.Errorf("list plan = %s", u)
	}
}

// io_ReadAll avoids importing io just for this helper.
func io_ReadAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			return buf, err
		}
	}
}

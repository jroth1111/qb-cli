package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type brokenDownload struct{}

func (brokenDownload) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestAttachableDownload(t *testing.T) {
	for _, scenario := range []string{"valid", "malformed pointer", "fetch failure"} {
		t.Run(scenario, func(t *testing.T) {
			saveUsable(t)
			const secret = "synthetic-signed-key"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/v3/") {
					pointer := "https://files.example.test/file?apikey=" + secret
					if scenario == "malformed pointer" {
						pointer += "\ninvalid"
					}
					_, _ = io.WriteString(w, pointer)
					return
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("credentials sent to file host")
				}
				if scenario == "fetch failure" {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				_, _ = io.WriteString(w, "attachment bytes")
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			out := filepath.Join(t.TempDir(), "attachment")
			res, err := DownloadAttachable(context.Background(), "7", out)
			if scenario == "valid" {
				if err != nil || res.Bytes != len("attachment bytes") {
					t.Fatalf("download: %v, %v", res, err)
				}
				got, err := os.ReadFile(out)
				if err != nil || string(got) != "attachment bytes" {
					t.Fatalf("file: %q, %v", got, err)
				}
			} else {
				if err == nil {
					t.Fatal("bad download succeeded")
				}
				if strings.Contains(err.Error(), secret) {
					t.Fatal("signed URL leaked through error")
				}
			}
		})
	}
}

func TestWriteDownloadPreservesDestinationOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		body io.Reader
		pdf  bool
	}{
		{"short PDF", strings.NewReader("%P"), true},
		{"HTML", strings.NewReader("<html>error</html>"), true},
		{"oversize", strings.NewReader("123456789"), false},
		{"interrupted", io.MultiReader(strings.NewReader("%PDF-"), brokenDownload{}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "existing")
			if err := os.WriteFile(out, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := writeDownload(tc.body, out, 8, tc.pdf); err == nil {
				t.Fatal("invalid download succeeded")
			}
			got, err := os.ReadFile(out)
			if err != nil || string(got) != "original" {
				t.Fatalf("destination changed: %q, %v", got, err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("temporary file leaked: %v, %v", entries, err)
			}
		})
	}
}

func TestWriteDownloadExactLimit(t *testing.T) {
	out := filepath.Join(t.TempDir(), "file")
	const body = "%PDF-1.7"
	n, err := writeDownload(strings.NewReader(body), out, int64(len(body)), true)
	if err != nil || n != len(body) {
		t.Fatalf("download = %d, %v", n, err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != body {
		t.Fatalf("saved bytes = %q, %v", got, err)
	}
}

func TestPDFDownloadEndpointsValidateBody(t *testing.T) {
	for _, endpoint := range []struct {
		name, path string
		run        func(context.Context, string, string) (int, error)
	}{
		{"invoice", "/api/v3/company/12345/invoice/7/pdf", func(ctx context.Context, id, out string) (int, error) {
			r, e := ReplayInvoicePDF(ctx, id, out)
			if e != nil {
				return 0, e
			}
			return r.Bytes, nil
		}},
		{"estimate", "/api/v3/company/12345/estimate/7/pdf", func(ctx context.Context, id, out string) (int, error) {
			r, e := ReplaySalesFormPDF(ctx, "Estimate", id, out)
			if e != nil {
				return 0, e
			}
			return r.Bytes, nil
		}},
		{"document", "/v3/documents/pdf", func(ctx context.Context, id, out string) (int, error) {
			r, e := ReplayDocumentPDF(ctx, id, out)
			if e != nil {
				return 0, e
			}
			return r.Bytes, nil
		}},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			for _, body := range []string{"", "x", "%P", "%PD", "%PDF", "oops", "<html>bad</html>", "%PDF-1.7 complete"} {
				t.Run(body, func(t *testing.T) {
					saveUsable(t)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method != http.MethodGet || r.URL.Path != endpoint.path {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						}
						_, _ = io.WriteString(w, body)
					}))
					defer srv.Close()
					interceptHTTP(t, srv.URL)
					out := filepath.Join(t.TempDir(), "file.pdf")
					n, err := endpoint.run(context.Background(), "7", out)
					if strings.HasPrefix(body, "%PDF-") {
						if err != nil || n != len(body) {
							t.Fatalf("valid PDF: %d, %v", n, err)
						}
					} else {
						if err == nil {
							t.Fatal("invalid PDF succeeded")
						}
						if _, e := os.Stat(out); !errors.Is(e, os.ErrNotExist) {
							t.Fatalf("invalid PDF created a file: %v", e)
						}
					}
				})
			}
		})
	}
}

// Benchmark the old buffering pattern against the streaming file writer.
func BenchmarkDownload(b *testing.B) {
	for _, size := range []int64{1 << 20, 16 << 20} {
		for _, buffered := range []bool{true, false} {
			name := "stream"
			if buffered {
				name = "buffer"
			}
			b.Run(fmt.Sprintf("%dMiB/%s", size>>20, name), func(b *testing.B) {
				out := filepath.Join(b.TempDir(), "file")
				b.ReportAllocs()
				b.SetBytes(size)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					r := io.LimitReader(zeroDownload{}, size)
					if buffered {
						data, err := io.ReadAll(r)
						if err != nil {
							b.Fatal(err)
						}
						if err := os.WriteFile(out, data, 0o600); err != nil {
							b.Fatal(err)
						}
					} else if _, err := writeDownload(r, out, size, false); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

type zeroDownload struct{}

func (zeroDownload) Read(p []byte) (int, error) { clear(p); return len(p), nil }

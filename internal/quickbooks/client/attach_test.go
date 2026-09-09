package client

import (
	"context"
	"testing"
)

func TestUploadAttachableValidation(t *testing.T) {
	t.Parallel()
	if _, err := UploadAttachable(context.Background(), "", []byte("x")); err == nil {
		t.Error("empty filename must fail before dialing")
	}
	if _, err := UploadAttachable(context.Background(), "../evil.txt", []byte("x")); err == nil {
		t.Error("path separators must fail before dialing")
	}
	if _, err := UploadAttachable(context.Background(), "a.txt", nil); err == nil {
		t.Error("empty content must fail before dialing")
	}
	if _, err := UploadAttachable(context.Background(), "a.txt", make([]byte, 101<<20)); err == nil {
		t.Error("over-cap content must fail before dialing")
	}
}

func TestSniffUploadType(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"r.pdf": "application/pdf", "i.png": "image/png", "p.jpg": "image/jpeg",
		"d.csv": "text/csv", "n.txt": "text/plain", "x.bin": "application/octet-stream",
		"UP.PDF": "application/pdf",
	}
	for in, want := range cases {
		if got := sniffUploadType(in); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}

func TestReplayInvoicePDFValidation(t *testing.T) {
	t.Parallel()
	if _, err := ReplayInvoicePDF(context.Background(), "", "/tmp/x.pdf"); err == nil {
		t.Error("empty id must fail before dialing")
	}
	if _, err := ReplayInvoicePDF(context.Background(), "7", ""); err == nil {
		t.Error("empty out path must fail before dialing")
	}
	if _, err := ReplayInvoicePDF(context.Background(), "  ", "/tmp/x.pdf"); err == nil {
		t.Error("blank id must fail before dialing")
	}
}

func TestParseAttachableResponseBothEnvelopes(t *testing.T) {
	t.Parallel()
	js := []byte(`{"AttachableResponse":[{"Attachable":{"Id":"1","FileName":"a.txt"}}]}`)
	if id, name := parseAttachableResponse(js); id != "1" || name != "a.txt" {
		t.Errorf("json: got %q %q", id, name)
	}
	xmlDoc := []byte(`<?xml version="1.0"?><IntuitResponse><AttachableResponse><Attachable><Id>2</Id><FileName>b.txt</FileName></Attachable></AttachableResponse></IntuitResponse>`)
	if id, name := parseAttachableResponse(xmlDoc); id != "2" || name != "b.txt" {
		t.Errorf("xml: got %q %q", id, name)
	}
	if id, _ := parseAttachableResponse([]byte("garbage")); id != "" {
		t.Errorf("garbage must yield empty id, got %q", id)
	}
}

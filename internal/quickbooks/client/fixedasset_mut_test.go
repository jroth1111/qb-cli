package client

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestFixedAssetDeleteRejectsMissingReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"financeDeleteAsset":null}}`)
	interceptHTTP(t, srv.URL)
	_, err := ReplayFixedAssetMutate(context.Background(), "delete", map[string]string{"id": "asset-1"})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

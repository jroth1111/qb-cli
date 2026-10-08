package client

import (
	"context"
	"strconv"
	"testing"
)

func TestPopulationAndVerifyWalkCappedResponses(t *testing.T) {
	for _, verify := range []bool{false, true} {
		t.Run(strconv.FormatBool(verify), func(t *testing.T) {
			t.Setenv("QB_HTTP_TRANSPORT", "")
			saveUsable(t)
			const total = 650
			capacity := 250
			if verify {
				capacity = 13 // More than forty pages must still be enumerated.
			}
			fs := newFeedServer(t, "204", new(total), func(start, size int) []rawTxn {
				if start >= total {
					return nil
				}
				return pageOf(min(size, capacity, total-start), func(i int) string {
					return "row-" + strconv.Itoa(start+i)
				})
			})
			interceptHTTP(t, fs.URL)
			if verify {
				got, err := VerifyFeed(context.Background(), "204")
				if err != nil || !got.Complete || got.Fetched != total {
					t.Fatalf("verification skipped capped rows: result=%+v err=%v", got, err)
				}
			} else {
				got, err := ReplayFeedComplete(context.Background(), "204", "PENDING", 0)
				if err != nil || !got.Complete || len(got.Transactions) != total {
					t.Fatalf("population skipped capped rows: result=%+v err=%v", got, err)
				}
			}
		})
	}
}

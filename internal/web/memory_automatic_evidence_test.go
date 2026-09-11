package web

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
)

func TestAutomaticMemoryEvidenceHTTPInspectsFirstOrdinaryRequest(t *testing.T) {
	f := newReceiptHTTPFixture(t)
	client := &fakeClient{steps: []fakeStep{{content: "The original marker is azurefolio."}}}
	runtime := f.runtime(f.reader, client)
	f.send(runtime, "What was my retrieval marker?")
	if len(client.reqs) != 1 {
		t.Fatalf("ordinary request used %d provider calls", len(client.reqs))
	}
	events, err := runtime.HistoryEvents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	answer := events[len(events)-1]
	rr := f.inspect(runtime, map[string]string{"answerId": string(answer.ID)})
	if rr.Code != http.StatusOK {
		t.Fatalf("inspection=%d %s", rr.Code, rr.Body.String())
	}
	var receipt receiptHTTPResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	wire, err := openrouter.RequestBytes(client.reqs[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.Requests) != 1 || receipt.Requests[0].RequestStatus != "completed" || receipt.Requests[0].RequestSHA256 != fmt.Sprintf("%x", sha256.Sum256(wire)) || receipt.Requests[0].SerializedBytes != int64(len(wire)) {
		t.Fatalf("first ordinary request receipt differs from actual provider bytes: %s", rr.Body.String())
	}
	found := false
	for _, item := range receipt.Requests[0].Evidence {
		found = found || item.Available && item.Reference.ClaimID == f.accepted.ClaimID
	}
	if !found {
		t.Fatalf("ordinary answer cannot inspect its original automatic evidence: %s", rr.Body.String())
	}
}

package eviedb
import("testing";"github.com/davidadel66/evie/internal/memory")
func TestRootMalformedCompoundFailsClosed(t *testing.T) {
 d:=[]memory.ReviewDependency{{CandidateID:"b",Field:"subject",FromCandidateID:"a",FromField:"subject"}}
 p:=memory.ReviewPreview{Version:"owner-review-preview-v5",Action:"accept",Candidates:[]memory.OwnerCandidate{{Ref:memory.CandidateRef{ID:"a"}},{Ref:memory.CandidateRef{ID:"b"}}},Dependencies:d,Effect:&memory.ReviewEffect{Version:"owner-review-effect-v5",Members:[]memory.ReviewEffect{{},{}},Claims:[]memory.ReviewClaimEffect{{},{}},Dependencies:d}}
 defer func(){if r:=recover();r!=nil {t.Fatalf("malformed compound panicked instead of failing closed: %v",r)}}()
 if err:=validateOwnerReviewEncoding(p);err==nil {t.Fatal("accepted malformed compound")}
}

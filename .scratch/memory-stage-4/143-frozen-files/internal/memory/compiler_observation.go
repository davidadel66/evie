package memory

// CompilerClockEvidencePolicy admits only the existing parameterless local
// clock observation in addition to owner assertions. Other generation policies
// remain independently pinned; owner-assertions-v1 never admits a tool source.
const CompilerClockEvidencePolicy = "owner-clock-observations-v2"
const LocalClockDisplayContract = "local-clock-display-v1"
const SemanticActorTool SemanticActor = "tool"
const AuthorityToolObservation SourceAuthority = "tool_observation"
const SourceTypeToolSucceeded SemanticSourceType = "tool_succeeded"

// CompilerObservation pins the original durable control ancestry. Its digest
// binds the exact recorded metadata, not a new operational clock record.
type CompilerObservation struct {
	Contract       string      `json:"contract"`
	RootID         EventID     `json:"root_id"`
	ExecutionID    ExecutionID `json:"execution_id"`
	CallID         string      `json:"call_id"`
	AncestrySHA256 string      `json:"ancestry_sha256"`
}

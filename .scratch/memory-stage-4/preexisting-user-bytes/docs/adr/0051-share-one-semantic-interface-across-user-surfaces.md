# Share one semantic interface across user surfaces

The REPL, conversational tools, and web UI use the same scope-bound Memory
Inspection behavior and prepare the same typed Memory Operation Proposals for
mutation. No surface edits semantic tables directly or invents its own approval
path. Focused model-facing tools adapt into one prepare/apply interface, while
replay remains a Kernel maintenance and test function rather than a Capability.
Stage 3 includes an eventless REPL inspection command and a basic read-only web
Memory tab as separate slices over that interface; a rich graph explorer is a
separate prototype-backed feature so visualization choices cannot distort or
block the Semantic Memory domain.

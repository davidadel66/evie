package main

// Frozen independent compact transport configuration; original prompts remain pinned.
const compactPromptSHA256 = "sha256:3158397c034891034c26bfa08c5cefe4970d89232882c8e054d8503a831ee672"
const compactSchemaSHA256 = "sha256:0d6c989ca4f31c55b72b0cca700076a143bc130c57c94dd189a697c461a06048"

// compact-v2 changes only the generator schema; projection and expansion stay compact-v1.
const compactV2PromptSHA256 = "sha256:86e61089a64e6e3490cc40a6e72e3ee65a3b434499f054d3bb22e225a0a3039b"
const compactV2SchemaSHA256 = "sha256:ad6ce8c0ea12f11345a3d421d710f718d9535ba24616858d78c2f207d4288c6e"

func isCompactWire(wire string) bool {
	return wire == "compact-v1" || wire == "compact-v2" || wire == "compact-v3"
}

func compactBudgetVersion(wire string) string {
	if !isCompactWire(wire) {
		return ""
	}
	return "pinned-qwen2-" + wire + "-byte-bound-v1"
}

func isQwenByteBudget(version string) bool {
	return version == "pinned-qwen2-bpe-byte-bound-v1" || version == compactBudgetVersion("compact-v1") || version == compactBudgetVersion("compact-v2") || version == compactBudgetVersion("compact-v3")
}

const compactCategoryVersion = "compact-category-v1"
const compactV3PromptSHA256 = "sha256:49ccd62ac64ed0d1d68787c69b41d0bde6c47bb2093ea336c1499a252d37fab9"
const compactV3SchemaSHA256 = compactV2SchemaSHA256
const compactCategoryPolicySHA256 = "sha256:73ca627813b1cc5ddedb8e8305c1f90fbbab4ff08162fe9e438716b168acbaaa"

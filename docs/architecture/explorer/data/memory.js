evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "memory",
  "nodes": [
    {
      "id": "pkg:internal/memory",
      "level": "component",
      "kind": "package",
      "title": "memory",
      "group": "memory",
      "summary": {
        "text": "Defines the memory types other packages share, from Semantic Memory objects to candidate review and background-pass batches.",
        "refs": [
          "code:internal/memory/semantic.go#SemanticID",
          "code:internal/memory/candidate_review.go",
          "code:internal/memory/background.go#BackgroundBatch"
        ]
      },
      "terms": [
        "Semantic Memory"
      ]
    },
    {
      "id": "pkg:internal/memorybackground",
      "level": "component",
      "kind": "package",
      "title": "memorybackground",
      "group": "memory",
      "summary": {
        "text": "Runs the always-on background memory pass, which extracts candidate memories from the owner's new messages and queues proposals for the owner's review.",
        "refs": [
          "code:internal/memorybackground/config.go",
          "code:internal/memorybackground/pass.go#Runner"
        ]
      }
    },
    {
      "id": "pkg:internal/memoryeval",
      "level": "component",
      "kind": "package",
      "title": "memoryeval",
      "group": "memory",
      "summary": {
        "text": "Defines the versioned fixture and report contracts shared by Semantic Memory evaluation stages.",
        "refs": [
          "code:internal/memoryeval/evaluation.go"
        ]
      },
      "terms": [
        "Semantic Memory"
      ]
    }
  ],
  "edges": [],
  "flows": []
}
);

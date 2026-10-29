evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "workflows",
  "nodes": [
    {
      "id": "pkg:internal/workflow",
      "level": "component",
      "kind": "package",
      "title": "workflow",
      "group": "runtime",
      "summary": {
        "text": "Loads, validates, hashes, reviews, and simulates reviewed Workflow Definitions.",
        "refs": [
          "code:internal/workflow/definition.go#Definition"
        ]
      },
      "terms": [
        "Workflow Definition"
      ]
    },
    {
      "id": "pkg:internal/workflowruntime",
      "level": "component",
      "kind": "package",
      "title": "workflowruntime",
      "group": "runtime",
      "summary": {
        "text": "The Workflow Runtime's single entry point: it activates reviewed definitions and starts, schedules, and recovers their runs.",
        "refs": [
          "code:internal/workflowruntime/runtime.go#Runtime"
        ]
      },
      "terms": [
        "Workflow Definition",
        "Workflow Run",
        "Standing Authority"
      ]
    },
    {
      "id": "pkg:internal/procedural",
      "level": "component",
      "kind": "package",
      "title": "procedural",
      "group": "runtime",
      "summary": {
        "text": "Evie's procedural Git repository: a private bare repository at ~/.evie/procedural that holds reviewed Workflow Definitions.",
        "refs": [
          "code:internal/procedural/repo.go#Repo"
        ]
      },
      "terms": [
        "Workflow Definition"
      ]
    }
  ],
  "edges": [],
  "flows": []
}
);

evieExplorer.register(
{
  "format": 1,
  "kind": "proposal",
  "id": "example-morning-digest",
  "feature": "morning-digest",
  "title": "Example: a morning digest",
  "status": "example",
  "spec": "doc:docs/architecture/explorer/README.md#design-proposals",
  "summary": {
    "text": "An example design proposal, not planned work: a morning digest that would summarise the owner’s open Task Trees and recent memory as one chat message, shown here so the overlay can be seen working.",
    "refs": [
      "doc:docs/architecture/explorer/README.md#design-proposals"
    ]
  },
  "nodes": [
    {
      "op": "add",
      "id": "pkg:internal/digest",
      "level": "component",
      "kind": "package",
      "title": "digest",
      "group": "runtime",
      "summary": {
        "text": "Would compose the morning digest from open Task Trees and recently recalled memory.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      },
      "does": [
        {
          "text": "Would gather the owner’s open Task Trees and the memory recalled in the last day, then post one summary turn.",
          "refs": [
            "node:pkg:internal/digest",
            "doc:docs/architecture/explorer/README.md#design-proposals"
          ]
        }
      ]
    },
    {
      "op": "add",
      "id": "container:digest-scheduler",
      "level": "container",
      "kind": "process",
      "title": "Digest scheduler",
      "summary": {
        "text": "Would be a background host inside evie serve that wakes once each morning to run the digest.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      }
    },
    {
      "op": "change",
      "id": "pkg:internal/agent",
      "summary": {
        "text": "Would also run the digest’s summary turn through the same turn loop as a chat turn.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      }
    },
    {
      "op": "change",
      "id": "container:serve",
      "summary": {
        "text": "Would also start the digest scheduler beside its other background hosts.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      }
    },
    {
      "op": "remove",
      "id": "pkg:internal/taskeval"
    }
  ],
  "edges": [
    {
      "op": "add",
      "from": "pkg:internal/digest",
      "to": "pkg:internal/task",
      "via": "import",
      "label": "reads open Task Trees"
    },
    {
      "op": "add",
      "from": "pkg:internal/digest",
      "to": "pkg:internal/memory",
      "via": "import",
      "label": "recalls recent memory"
    },
    {
      "op": "add",
      "from": "pkg:internal/digest",
      "to": "pkg:internal/agent",
      "via": "import",
      "label": "posts the digest as a turn"
    },
    {
      "op": "add",
      "from": "container:serve",
      "to": "container:digest-scheduler",
      "via": "process",
      "label": "starts each morning",
      "refs": [
        "doc:docs/architecture/explorer/README.md#design-proposals"
      ]
    },
    {
      "op": "add",
      "from": "container:digest-scheduler",
      "to": "container:evie-db",
      "via": "sql",
      "label": "reads tasks and memory",
      "refs": [
        "doc:docs/architecture/explorer/README.md#design-proposals"
      ]
    },
    {
      "op": "change",
      "from": "pkg:internal/agent",
      "to": "pkg:internal/task",
      "via": "import",
      "label": "also marks digest turns",
      "refs": [
        "doc:docs/architecture/explorer/README.md#design-proposals"
      ]
    },
    {
      "op": "remove",
      "from": "pkg:internal/plugins",
      "to": "pkg:internal/workspacetabs",
      "via": "import"
    }
  ],
  "flows": [
    {
      "op": "add",
      "id": "example-digest-run",
      "title": "Example: morning digest run",
      "summary": {
        "text": "Each morning the scheduler would ask the digest for a summary, which reads tasks and memory and posts one turn.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      },
      "trigger": {
        "text": "The digest scheduler’s morning tick.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      },
      "participants": [
        "container:digest-scheduler",
        "pkg:internal/digest",
        "pkg:internal/task",
        "pkg:internal/memory",
        "pkg:internal/agent"
      ],
      "steps": [
        {
          "from": "container:digest-scheduler",
          "to": "pkg:internal/digest",
          "label": "Ask for this morning’s digest",
          "kind": "call",
          "via": "call",
          "refs": [
            "node:container:digest-scheduler"
          ]
        },
        {
          "from": "pkg:internal/digest",
          "to": "pkg:internal/task",
          "label": "List open Task Trees",
          "kind": "call",
          "via": "call",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/task",
          "to": "pkg:internal/digest",
          "label": "Open trees and their next steps",
          "kind": "return",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/digest",
          "to": "pkg:internal/memory",
          "label": "Recall memory from the last day",
          "kind": "call",
          "via": "call",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/memory",
          "to": "pkg:internal/digest",
          "label": "Recalled memories",
          "kind": "return",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/digest",
          "to": "pkg:internal/digest",
          "label": "Draft the summary",
          "kind": "call",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/digest",
          "to": "pkg:internal/agent",
          "label": "Post the digest as one summary turn",
          "kind": "async",
          "via": "call",
          "refs": [
            "node:pkg:internal/digest"
          ]
        },
        {
          "from": "pkg:internal/digest",
          "to": "pkg:internal/agent",
          "label": "The owner reads it in the chat like any other reply",
          "kind": "note",
          "refs": [
            "doc:docs/architecture/explorer/README.md#design-proposals"
          ]
        }
      ]
    },
    {
      "op": "change",
      "id": "subagent-run",
      "summary": {
        "text": "Would also let a digest turn delegate a research child, through the same bounded child attempts.",
        "refs": [
          "doc:docs/architecture/explorer/README.md#design-proposals"
        ]
      }
    },
    {
      "op": "remove",
      "id": "approval-handshake"
    }
  ],
  "requirements": [
    {
      "text": "Loading this example through the page’s proposal parameter shows a toggle; turning it on restyles what it adds, changes, and removes.",
      "refs": [
        "doc:cmd/evie/docs/active/system-explorer.spec.md#acceptance-criteria"
      ]
    },
    {
      "text": "Every node, edge, and flow operation kind appears at least once, so each style can be checked by eye.",
      "refs": [
        "doc:cmd/evie/docs/active/system-explorer.spec.md#viewer-behaviour"
      ]
    }
  ],
  "decisions": [
    {
      "text": "The example lives in examples/ and is not listed in the manifest, so it loads only on request.",
      "refs": [
        "doc:docs/architecture/explorer/README.md#files"
      ]
    },
    {
      "text": "Design proposals are data the page overlays, never changes to the page itself.",
      "refs": [
        "doc:cmd/evie/docs/active/system-explorer.decisions.md"
      ]
    }
  ]
}
);

evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "runtime",
  "nodes": [
    {
      "id": "pkg:internal/agent",
      "level": "component",
      "kind": "package",
      "title": "agent",
      "group": "runtime",
      "summary": {
        "text": "Runs one conversation Session's model-and-tools loop, independent of any frontend.",
        "refs": [
          "code:internal/agent/agent.go#Session"
        ]
      }
    },
    {
      "id": "pkg:internal/tools",
      "level": "component",
      "kind": "package",
      "title": "tools",
      "group": "runtime",
      "summary": {
        "text": "Holds every tool the model can call, one file per tool, with the registry and dispatcher that run them in the chat loop.",
        "refs": [
          "code:internal/tools/registry.go#Tool"
        ]
      }
    },
    {
      "id": "pkg:internal/plugins",
      "level": "component",
      "kind": "package",
      "title": "plugins",
      "group": "runtime",
      "summary": {
        "text": "The Kernel's composition seam for compiled first-party plugins: its Manager resolves a fresh toolset from the enabled plugins for each new session.",
        "refs": [
          "code:internal/plugins/manager.go#Manager"
        ]
      },
      "terms": [
        "Plugin",
        "First-party Plugin",
        "Kernel"
      ]
    },
    {
      "id": "pkg:internal/composition",
      "level": "component",
      "kind": "package",
      "title": "composition",
      "group": "runtime",
      "summary": {
        "text": "Defines the content-free Composition Receipt that durably identifies what one session was composed from.",
        "refs": [
          "code:internal/composition/receipt.go#Receipt"
        ]
      },
      "terms": [
        "Composition Receipt"
      ]
    },
    {
      "id": "pkg:internal/subagents",
      "level": "component",
      "kind": "package",
      "title": "subagents",
      "group": "runtime",
      "summary": {
        "text": "Supervises bounded foreground child agents inside the Kernel.",
        "refs": [
          "code:internal/subagents/supervisor.go#Supervisor"
        ]
      },
      "terms": [
        "Kernel"
      ]
    },
    {
      "id": "pkg:internal/delegation",
      "level": "component",
      "kind": "package",
      "title": "delegation",
      "group": "runtime",
      "summary": {
        "text": "Defines the Kernel's durable execution records for delegated work, kept separate from Tasks.",
        "refs": [
          "code:internal/delegation/types.go"
        ]
      },
      "terms": [
        "Kernel",
        "Task"
      ]
    },
    {
      "id": "pkg:internal/task",
      "level": "component",
      "kind": "package",
      "title": "task",
      "group": "runtime",
      "summary": {
        "text": "Defines the Kernel-owned contract for durable Tasks.",
        "refs": [
          "code:internal/task/task.go#Task"
        ]
      },
      "terms": [
        "Task",
        "Task Tree"
      ]
    },
    {
      "id": "pkg:internal/taskeval",
      "level": "component",
      "kind": "package",
      "title": "taskeval",
      "group": "runtime",
      "summary": {
        "text": "Derives a stable, content-free evidence baseline for evaluating durable Tasks.",
        "refs": [
          "code:internal/taskeval/evaluation.go#Report"
        ]
      },
      "terms": [
        "Task"
      ]
    },
    {
      "id": "pkg:internal/untrusted",
      "level": "component",
      "kind": "package",
      "title": "untrusted",
      "group": "runtime",
      "summary": {
        "text": "Fences text Evie did not author, such as web pages and transcripts, so it reads as data rather than instructions.",
        "refs": [
          "code:internal/untrusted/frame.go#Frame"
        ]
      }
    },
    {
      "id": "pkg:internal/repoinstructions",
      "level": "component",
      "kind": "package",
      "title": "repoinstructions",
      "group": "runtime",
      "summary": {
        "text": "Loads only the designated root repository guidance of a Workspace folder.",
        "refs": [
          "code:internal/repoinstructions/load.go#Load"
        ]
      }
    }
  ],
  "edges": [],
  "flows": []
}
);

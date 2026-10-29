evieExplorer.register(
{
  "format": 1,
  "kind": "content",
  "owner": "intelligence",
  "nodes": [
    {
      "id": "pkg:internal/openrouter",
      "level": "component",
      "kind": "package",
      "title": "openrouter",
      "group": "intelligence",
      "summary": {
        "text": "A minimal client for OpenRouter's Chat Completions and Responses APIs.",
        "refs": [
          "code:internal/openrouter/schema.go#Client",
          "code:internal/openrouter/client.go"
        ]
      }
    },
    {
      "id": "pkg:internal/localembedding",
      "level": "component",
      "kind": "package",
      "title": "localembedding",
      "group": "intelligence",
      "summary": {
        "text": "Implements the local-only Ollama embedding protocol that memory embedding uses.",
        "refs": [
          "code:internal/localembedding/client.go#Client",
          "code:internal/localembedding/client.go#ErrDisabled"
        ]
      }
    },
    {
      "id": "pkg:internal/localextractor",
      "level": "component",
      "kind": "package",
      "title": "localextractor",
      "group": "intelligence",
      "summary": {
        "text": "Implements the explicitly configured, pinned local compiler transport over Ollama.",
        "refs": [
          "code:internal/localextractor/ollama.go#Ollama"
        ]
      }
    }
  ],
  "edges": [],
  "flows": []
}
);

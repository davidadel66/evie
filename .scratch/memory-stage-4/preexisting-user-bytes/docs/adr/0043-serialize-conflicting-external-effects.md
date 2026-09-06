# Serialize conflicting external effects

Workflow Runs with different Run Keys may calculate and read concurrently, but
external-effect capabilities declare Resource Conflict Keys and Evie serializes
writes sharing a key. Connectors also use conditional or idempotent mutations
where available. When a provider cannot protect the resource, workflow review
must require broader single-file execution or a durable human interruption.

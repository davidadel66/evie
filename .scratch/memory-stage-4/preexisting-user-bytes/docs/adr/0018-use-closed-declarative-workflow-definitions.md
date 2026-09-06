# Use closed declarative Workflow Definitions

Each Procedural Workflow uses a versioned `workflow.yaml` with a closed schema,
a human-readable `README.md`, and separate Markdown prompt files for AI Nodes.
Activation validates and canonicalizes the YAML into the deterministic content
that is hashed and pinned. Workflow Definitions cannot embed arbitrary Go,
Python, JavaScript, shell, or dynamically generated executable code. The first
runtime does not support workflow-to-workflow calls; definitions initially
reuse shared deterministic capability nodes, while pinned subworkflow calls
remain an explicit later extension.

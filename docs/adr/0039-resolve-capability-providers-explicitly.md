# Resolve capability providers explicitly

Agent Presets, Model Policies, and Workflow Definitions select capability
providers by canonical identity and compatible contract. Evie never chooses a
model, sandbox, tool, or other provider because it loaded first. This rule also
defines the later seams for model, sandbox, subagent, memory, workflow, and UI
providers without requiring those roles in the first implementation.

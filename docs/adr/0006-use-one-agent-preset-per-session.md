# Use one Agent Preset per session

Each Evie session is composed from one explicit or default Agent Preset whose
plugins, tools, instructions, and skills remain fixed after the session begins.
The default preset is `standard`; specialized work may use presets such as
`cairos-kitchen`, created and configured through Evie sessions. Mid-session
capability selection and preset switching are deferred until actual use shows
that fixed composition is too restrictive. This adopts DeepSeek Harness's
inspectable session-composition model and supersedes ADR-0001's per-request
capability selection.

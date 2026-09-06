# Migrate unfinished work with Successor Runs

An incompatible unfinished Workflow Run is never rewritten to claim it used a
new definition or runtime. Explicit migration creates a linked Successor Run
under an approved new Workflow Definition and carries forward only validated
state and durable Effect Receipts. The predecessor remains intact in the audit
history, and inherited effect identities prevent completed actions from being
performed twice.

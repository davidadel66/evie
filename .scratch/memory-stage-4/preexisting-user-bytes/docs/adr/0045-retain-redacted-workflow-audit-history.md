# Retain redacted workflow audit history

Evie retains a complete local audit trail by default, including accepted
inputs, model proposals, human corrections, effect intents, provider responses,
and receipts. Credentials and raw tokens are never recorded, Workflow
Definitions classify sensitive fields for redaction, and each Workspace may
define retention and export rules. Deletion is an explicit, auditable operation
rather than an incidental consequence of ordinary cleanup.

# Distinguish credential rotation from resource changes

Refreshing or rotating credentials behind the same Connection ID does not
change an approved workflow and does not require Workflow Approval again.
Changing a Connection ID, external account, location, spreadsheet, recipient
set, or another resource bounded by Standing Authority creates a new Workflow
Definition version and requires review. The Kernel, rather than procedural
assets or plugins, continues to own credential material.

# Workflow approval grants bounded standing authority

One Workflow Approval accepts both an exact Workflow Definition version and its
explicitly declared Standing Authority for future runs, so in-scope actions do
not repeatedly prompt the owner. The Kernel enforces the approved accounts,
resources, operations, schedules, recipients, and limits and stops any attempted
expansion. Executable logic, prompts, capabilities, resources, schedules, or
authority changes create a new version requiring approval; documentation-only
edits do not, and existing runs stay pinned to the version they started with.

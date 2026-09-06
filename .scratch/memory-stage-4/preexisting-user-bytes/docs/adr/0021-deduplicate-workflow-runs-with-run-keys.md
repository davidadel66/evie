# Deduplicate Workflow Runs with Run Keys

Each Workflow Definition declares how to derive a deterministic Run Key from
its logical input, such as a Cairo's Kitchen location and business date. Manual
and scheduled starts use the same key. Evie rejects a second active run and
remembers completed keys so a logical execution cannot silently happen twice;
rerunning one requires an explicit override that is durably recorded.

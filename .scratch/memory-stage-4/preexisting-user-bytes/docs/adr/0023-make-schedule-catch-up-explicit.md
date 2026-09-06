# Make schedule catch-up and overlap explicit

Each Workflow Schedule declares an IANA timezone and a bounded Catch-up Policy
instead of inheriting host-local time or one universal missed-run behavior.
The Cairo's Kitchen tip workflow creates one recovery run for each missing
business date. Runs with different Run Keys may progress independently, while
Run Key deduplication prevents overlapping manual and scheduled execution of
the same logical work.

# Run the daily finance job as a scheduled workflow, not a cron job

The daily finance sync and categorize ran as a launchd cron job that
shelled out to a separately installed `finance` binary, and that binary
drifted two months behind the package compiled into Evie until a known,
fixed bug wedged two banks. We decided that unattended feature operations
run as reviewed, scheduled Workspace workflows inside `evie serve`, with
the Feature Plugin providing the operations as Workflow Effects, and that
`evie serve` itself is kept alive by a launchd LaunchAgent. This gives one
deployment, owner approval of the Standing Authority, catch-up after
downtime and durable failure notifications, at the cost of the scheduler
living inside Evie rather than in launchd. The cron tool remains for plain
shell commands; it is no longer the way a plugin's own operations are
scheduled. Finance is the first production effect provider.

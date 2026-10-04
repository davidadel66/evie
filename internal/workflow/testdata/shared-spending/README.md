# Shared spending

Split one month's spending evenly between people in whole cents. The run
reads the month's posted cash flow from the local finance database (the
`finance.month_cash_flow` read on the `finance-local` connection's `ledger`),
splits its outflow evenly, and reports either the amount each person pays or
the cents left over. The run key is the month and the number of people, so
one month is split once for a given group.

This is the test example for capability reads: a read, deterministic
functions and a branch. The Workspace must allow `finance-local=ledger`
before the definition can be approved.

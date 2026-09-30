# Connected accounts and simplified navigation

Authorized by David on 2026-09-21, following workspace setup and session archive.

## Outcome and acceptance

The sidebar removes its top-level New chat and Workspaces buttons. Data remains,
followed by one Workspaces heading, its creation plus, each workspace's session
plus, saved conversations, and Settings. Existing workspace pages and session
creation remain available through the workspace list and existing empty-state
flows; no saved conversations are removed.

Data > Spending shows connected banks and their authorized accounts above the
Overview/Transactions controls. Institution groups show account names, optional
masked digits, and type/subtype without balances, full account numbers, provider
IDs, or credentials. Every saved bank connection appears, including older links
whose account names have not been fetched and accounts without transactions.
Unknown inventory is explicit rather than presented as zero accounts.

Opening Spending reads local cached inventory. Refresh accounts explicitly asks
Plaid for its account inventory and saves it; it does not sync transactions or
request fresh balances from a bank. Partial failure preserves each failed bank's
last saved inventory and reports incomplete refresh. Fetch latest retains its
existing explicit transaction-sync behavior.

Add account uses the existing Plaid Hosted Link integration. It opens an ordinary
browser tab and provides a visible Continue with Plaid link if a popup is blocked
or closed. The owner completes institution authentication and consent in Plaid.
Evie polls only a started or resumed connection and reloads saved accounts after
it completes. Closing a browser tab or arriving at a redirect is not evidence of
success. No embedded browser SDK, new production dependency, or credential-entry
form is added to Evie.

## Service and persistence boundaries

Reuse `internal/finance` and the canonical `~/.finance/finance.db`. The existing
CLI flow's Hosted Link create/get/public-token exchange operations are shared
with the web service. Provider errors and token-bearing responses are never
printed or returned to the browser. A hosted URL is intentionally browser-bound;
the server accepts only HTTPS URLs on `secure.plaid.com` without credentials.

Local account cache and connection-attempt tables are created on an explicit
writable operation. Passive reads tolerate older databases without those tables,
do not create missing storage, and do not perform bank inventory requests.
Account records are keyed internally by their Item/account identity, never by
names or masks; public responses use opaque local identifiers. Nullable masks
remain absent. Successful inventory replacement is atomic per institution.

One pending hosted flow is retained with an opaque local handle and expiration;
reloading or restarting Evie can resume it. Completion is serialized and uses
Plaid session item-add results. Public/access/link tokens stay server-side. Save
credentials immediately after a successful exchange, before fetching metadata;
failed metadata fetch leaves a visible, refreshable connection. Persist exchange
state to prevent replay of a single-use public token. A crash or ambiguous
provider response during exchange can require starting again; do not claim an
idempotent provider guarantee that Plaid does not provide.

Cancel stops an unfinished local connection flow; it never removes a saved Item
or financial history. Expired, cancelled, linked, and failed flows stop polling.
Unmounted components cancel reads and timers. Mutation ambiguity is resolved by
reloading canonical saved state, not by treating a browser abort as rollback.

New initial Link sessions can create separate Items for the same institution.
Do not merge by institution name, overwrite another Item, or silently unlink a
connection. Repair/update mode, adding newly authorized accounts to an existing
Item, unlinking, and duplicate-Item cleanup are outside this requested UI slice.

The HTTP boundary uses existing owner-management method/host/origin/content-type
guards, strict bounded JSON, safe errors, no-store and no-referrer responses,
and bounded contexts. Clients supply only the locally issued attempt handle;
they cannot provide access/public tokens, provider URLs, or database paths.

## Verification

Test old/missing storage, all linked institutions, account names and nullable
masks, accounts without transactions, partial inventory refresh, replacement
atomicity, start/reuse/resume/expiry, completion and cancellation races, token
privacy, safe provider failures, and single-use exchange recovery. HTTP tests
prove invalid requests never invoke the finance service. UI checks cover the
simplified sidebar, empty/unknown/loaded/error account views, blocked-popup
fallback, pending/terminal link states, explicit-only inventory fetches, keyboard
access, and narrow-screen containment.

Use fake provider clients and temporary databases for deterministic tests, and
a disposable HTTP browser fixture for UI checks. Do not complete live financial
account authentication on the owner's behalf. Run UI Vitest and the repository's
`./scripts/verify-change.sh`; state the live-provider verification boundary.

### Verified on 2026-09-22

- `GOFLAGS=-p=2 ./scripts/verify-change.sh` passed: full Go tests and vet,
  UI lint, TypeScript/production build, and staged/unstaged whitespace checks.
  The five existing Fast Refresh lint warnings and Vite's large-chunk warning
  remain; no required check was skipped.
- `npx vitest run` from `internal/web/ui` passed all 365 tests in 60 files.
- `go test -race ./internal/finance -count=1` passed, including service,
  persistence, CLI, and local HTTP Plaid SDK fixture tests.
- `go test -p 2 -race ./internal/web -run '^TestSpendingAccountsHTTP' -count=1`
  passed the new HTTP boundary tests.
- The opt-in `TestSpendingAccountsBrowserFixture` passed with disposable data.
  Browser checks covered navigation, known and unknown accounts, explicit
  refresh, resumed completion, cancellation, status errors, recovery controls,
  and a 390px viewport without horizontal overflow. Initial view counters
  showed only local reads. Explicit recovery reloaded both account inventory
  and the local spending report without syncing transactions.
- `go build -p 2 -o /Users/davidboktor/go/bin/.evie-rebuild-accounts ./cmd/evie`
  passed. The installed service was restarted with its existing environment
  after consistent database and binary backups. Its served UI matched the
  built bundle, and all 23 sessions, 373 events, and 3 workspaces were unchanged.
- One explicit live account-inventory refresh succeeded for all 3 saved bank
  connections. The live UI rendered all 5 accounts. Credentials, transaction
  cursors, transactions, and category allocations were unchanged.

Manual demonstration: open Data > Spending to see the saved account groups;
choose Refresh accounts to refresh their metadata, or Add account to open
Plaid. The owner completes new bank authentication. A new live connection and
provider OAuth consent were not exercised; deterministic provider fixtures cover
the integration, and the live check covers existing authorized inventory only.

Review entry points are `SpendingAccounts.tsx`, the strict routes in
`internal/web/spending_accounts.go`, and the inventory/link services in
`internal/finance/spending_accounts.go` and `spending_account_link.go`.

## Primary references

- [Plaid Hosted Link](https://plaid.com/docs/link/hosted-link/): hosted browser
  flow and server-side completion via link session results.
- [Accounts API](https://plaid.com/docs/api/accounts/#accountsget): cached
  authorized account inventory, nullable masks, and account identities.
- [Duplicate Items](https://plaid.com/docs/link/duplicate-items/): initial Link
  creates new Items; update mode and deduplication have separate contracts.

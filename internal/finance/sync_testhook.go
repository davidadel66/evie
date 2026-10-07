package finance

import (
	"context"
	"database/sql"

	"github.com/plaid/plaid-go/v43/plaid"
)

// FakeSyncItemsForTest replaces the per-bank sync with fn for tests in
// other packages, which cannot reach runSyncItem. fn receives the item ID
// and returns the counts and error one bank's sync would. The returned
// function restores the real sync.
func FakeSyncItemsForTest(fn func(itemID string) (added, modified, removed int, err error)) (restore func()) {
	original := runSyncItem
	runSyncItem = func(_ context.Context, _ *plaid.APIClient, _ *sql.DB, item syncItem) (int, int, int, error) {
		return fn(item.itemID)
	}
	return func() { runSyncItem = original }
}

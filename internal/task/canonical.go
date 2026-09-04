package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// IdempotencySHA256 returns the content-free stable identity stored with Task
// mutation and coordination evidence.
func IdempotencySHA256(key IdempotencyKey) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

// CanonicalCreateRequestSHA256 returns the stable request identity used by
// durable Task idempotency after the trusted runtime resolves the target scope.
func CanonicalCreateRequestSHA256(input CreateInput, scope Scope) (string, error) {
	if input.Focus {
		return canonicalMutationSHA256(struct {
			Version                int    `json:"version"`
			Operation              string `json:"operation"`
			Scope                  Scope  `json:"scope"`
			Title                  string `json:"title"`
			Description            string `json:"description"`
			Priority               int    `json:"priority"`
			DueDate                string `json:"due_date"`
			ParentID               ID     `json:"parent_id"`
			ExpectedParentRevision uint64 `json:"expected_parent_revision"`
			Focus                  bool   `json:"focus"`
		}{2, string(OperationCreate), scope, input.Title, input.Description, input.Priority, input.DueDate,
			input.ParentID, input.ExpectedParentRevision, true})
	}
	return canonicalMutationSHA256(struct {
		Version                int    `json:"version"`
		Operation              string `json:"operation"`
		Scope                  Scope  `json:"scope"`
		Title                  string `json:"title"`
		Description            string `json:"description"`
		Priority               int    `json:"priority"`
		DueDate                string `json:"due_date"`
		ParentID               ID     `json:"parent_id"`
		ExpectedParentRevision uint64 `json:"expected_parent_revision"`
	}{1, string(OperationCreate), scope, input.Title, input.Description, input.Priority, input.DueDate,
		input.ParentID, input.ExpectedParentRevision})
}

// CanonicalDecomposeRequestSHA256 returns the stable request identity used by
// durable Task decomposition idempotency.
func CanonicalDecomposeRequestSHA256(id ID, input DecomposeInput) (string, error) {
	return canonicalMutationSHA256(struct {
		Version          int          `json:"version"`
		Operation        string       `json:"operation"`
		Scope            Scope        `json:"scope"`
		TaskID           ID           `json:"task_id"`
		ExpectedRevision uint64       `json:"expected_revision"`
		Children         []ChildInput `json:"children"`
	}{1, string(OperationDecompose), ScopeGlobal, id, input.ExpectedRevision, input.Children})
}

// CanonicalUpdateRequestSHA256 returns the stable request identity used by
// durable Task update idempotency, preserving patch-field presence.
func CanonicalUpdateRequestSHA256(id ID, input UpdateInput) (string, error) {
	title, titleSet := pointerValue(input.Title)
	description, descriptionSet := pointerValue(input.Description)
	priority, prioritySet := pointerValue(input.Priority)
	dueDate, dueDateSet := pointerValue(input.DueDate)
	status, statusSet := pointerValue(input.Status)
	if input.ResultSummary == nil {
		return canonicalMutationSHA256(struct {
			Version          int    `json:"version"`
			Operation        string `json:"operation"`
			Scope            Scope  `json:"scope"`
			TaskID           ID     `json:"task_id"`
			ExpectedRevision uint64 `json:"expected_revision"`
			TitleSet         bool   `json:"title_set"`
			Title            string `json:"title"`
			DescriptionSet   bool   `json:"description_set"`
			Description      string `json:"description"`
			PrioritySet      bool   `json:"priority_set"`
			Priority         int    `json:"priority"`
			DueDateSet       bool   `json:"due_date_set"`
			DueDate          string `json:"due_date"`
			StatusSet        bool   `json:"status_set"`
			Status           Status `json:"status"`
		}{
			1, string(OperationUpdate), ScopeGlobal, id, input.ExpectedRevision,
			titleSet, title, descriptionSet, description, prioritySet, priority,
			dueDateSet, dueDate, statusSet, status,
		})
	}
	resultSummary, resultSummarySet := pointerValue(input.ResultSummary)
	return canonicalMutationSHA256(struct {
		Version          int    `json:"version"`
		Operation        string `json:"operation"`
		Scope            Scope  `json:"scope"`
		TaskID           ID     `json:"task_id"`
		ExpectedRevision uint64 `json:"expected_revision"`
		TitleSet         bool   `json:"title_set"`
		Title            string `json:"title"`
		DescriptionSet   bool   `json:"description_set"`
		Description      string `json:"description"`
		PrioritySet      bool   `json:"priority_set"`
		Priority         int    `json:"priority"`
		DueDateSet       bool   `json:"due_date_set"`
		DueDate          string `json:"due_date"`
		ResultSummarySet bool   `json:"result_summary_set"`
		ResultSummary    string `json:"result_summary"`
		StatusSet        bool   `json:"status_set"`
		Status           Status `json:"status"`
	}{
		2, string(OperationUpdate), ScopeGlobal, id, input.ExpectedRevision,
		titleSet, title, descriptionSet, description, prioritySet, priority,
		dueDateSet, dueDate, resultSummarySet, resultSummary, statusSet, status,
	})
}

// CanonicalCoordinationRequestSHA256 returns the stable request identity for
// claim and release operations.
func CanonicalCoordinationRequestSHA256(id ID, operation Operation) (string, error) {
	return canonicalMutationSHA256(struct {
		Version   int       `json:"version"`
		Operation Operation `json:"operation"`
		Scope     Scope     `json:"scope"`
		TaskID    ID        `json:"task_id"`
	}{1, operation, ScopeGlobal, id})
}

func pointerValue[T any](value *T) (T, bool) {
	if value == nil {
		var zero T
		return zero, false
	}
	return *value, true
}

func canonicalMutationSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode canonical Task mutation: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

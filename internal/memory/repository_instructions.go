package memory

import "errors"

var ErrRepositoryInstructionsChanged = errors.New("Repository instruction settings changed; refresh and try again")

// Repository instruction records are operational context, never personal memory.
type RepositoryInstructionSettings struct {
	Enabled  bool  `json:"enabled"`
	Revision int64 `json:"revision"`
}
type RepositoryInstructionSnapshot struct {
	WorkspaceID WorkspaceID                   `json:"workspaceId"`
	SessionID   SessionID                     `json:"sessionId,omitempty"`
	TurnID      EventID                       `json:"turnId,omitempty"`
	Folder      WorkspaceFolder               `json:"folder"`
	Settings    RepositoryInstructionSettings `json:"settings"`
	Status      string                        `json:"status"`
	File        string                        `json:"file,omitempty"`
	Text        string                        `json:"text,omitempty"`
	SHA256      string                        `json:"sha256,omitempty"`
	Detail      string                        `json:"detail,omitempty"`
	CapturedAt  string                        `json:"capturedAt"`
	Prepared    bool                          `json:"prepared,omitempty"`
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

var (
	errPresetInvalid                = errors.New("Agent Preset validation failed")
	errSessionIDInvalid             = errors.New("invalid_session_id: session ID is required")
	errSessionNotFound              = errors.New("session_not_found: session was not found")
	errSessionInspectionUnavailable = errors.New("session_inspection_unavailable: session inspection is unavailable")
)

type managementReceiptInspector interface {
	GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error)
	GetCompatibilityResolutions(context.Context, memory.SessionID) ([]composition.CompatibilityResolution, error)
}

func runManagementCommand(
	ctx context.Context,
	args []string,
	out io.Writer,
	manager *plugins.Manager,
	receipts managementReceiptInspector,
) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	encode := func(value any) error {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	switch args[0] {
	case "plugins":
		if len(args) == 2 && args[1] == "list" {
			inspection, err := manager.InspectContext(ctx)
			if err != nil {
				return true, err
			}
			return true, encode(inspection)
		}
		if len(args) == 3 && (args[1] == "enable" || args[1] == "disable") {
			transition, err := manager.SetEnabledWithTransition(ctx, plugins.PluginID(args[2]), args[1] == "enable")
			if err != nil {
				return true, err
			}
			return true, encode(transition)
		}
		return true, errors.New("usage: evie plugins list|enable <id>|disable <id>")
	case "presets":
		if len(args) == 2 && args[1] == "list" {
			presets, err := manager.InspectPresetsContext(ctx)
			if err != nil {
				return true, err
			}
			return true, encode(struct {
				Presets []plugins.PresetInspection `json:"presets"`
			}{presets})
		}
		if len(args) == 3 && args[1] == "validate" {
			report, err := manager.ValidatePresetContext(ctx, plugins.PresetID(args[2]))
			if err != nil {
				return true, err
			}
			if err := encode(report); err != nil {
				return true, err
			}
			if !report.Valid {
				return true, errPresetInvalid
			}
			return true, nil
		}
		return true, errors.New("usage: evie presets list|validate <id>")
	case "sessions":
		if len(args) != 3 || args[1] != "inspect" {
			return true, errors.New("usage: evie sessions inspect <session-id>")
		}
		if receipts == nil {
			return true, errors.New("session receipt storage is unavailable")
		}
		sessionID := memory.SessionID(args[2])
		if strings.TrimSpace(string(sessionID)) == "" {
			return true, errSessionIDInvalid
		}
		receipt, err := receipts.GetCompositionReceipt(ctx, sessionID)
		if err != nil {
			if errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
				return true, errSessionNotFound
			}
			return true, errSessionInspectionUnavailable
		}
		resolutions, err := receipts.GetCompatibilityResolutions(ctx, sessionID)
		if err != nil {
			if errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
				return true, errSessionNotFound
			}
			return true, errSessionInspectionUnavailable
		}
		return true, encode(struct {
			SessionID                memory.SessionID                      `json:"sessionId"`
			Receipt                  composition.Receipt                   `json:"receipt"`
			CompatibilityResolutions []composition.CompatibilityResolution `json:"compatibilityResolutions"`
		}{sessionID, receipt, resolutions})
	default:
		return false, nil
	}
}

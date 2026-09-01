// Package composition owns the Kernel's content-free durable identity for one
// session composition. It is deliberately independent of providers and
// persistence so both may consume the same closed receipt schema.
package composition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
)

const FormatVersion = 1

type InstructionReference struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

type ConfigurationReferenceKind string

const ConfigurationConnection ConfigurationReferenceKind = "connection"

type ConfigurationReference struct {
	Kind ConfigurationReferenceKind `json:"kind"`
	ID   string                     `json:"id"`
}

type PresetIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Provider struct {
	ID                    string `json:"id"`
	ImplementationVersion string `json:"implementation_version"`
}

type Capability struct {
	ID              string `json:"id"`
	ProviderID      string `json:"provider_id"`
	ContractVersion string `json:"contract_version"`
	SchemaSHA256    string `json:"schema_sha256"`
}

type ToolSchema struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type WarningCode string

const (
	WarningProviderNotCompiled     WarningCode = "provider_not_compiled"
	WarningProviderDisabled        WarningCode = "provider_disabled"
	WarningProviderStopped         WarningCode = "provider_stopped"
	WarningProviderWaiting         WarningCode = "provider_waiting"
	WarningProviderFailed          WarningCode = "provider_failed"
	WarningCapabilityNotExposed    WarningCode = "capability_not_exposed"
	WarningContractIncompatible    WarningCode = "contract_incompatible"
	WarningProviderContractInvalid WarningCode = "provider_contract_invalid"
)

type Warning struct {
	Code         WarningCode `json:"code"`
	CapabilityID string      `json:"capability_id"`
	ProviderID   string      `json:"provider_id"`
}

type Receipt struct {
	FormatVersion int                      `json:"format_version"`
	Preset        PresetIdentity           `json:"preset"`
	EvieVersion   string                   `json:"evie_version"`
	Providers     []Provider               `json:"providers"`
	Capabilities  []Capability             `json:"capabilities"`
	ToolSchemas   []ToolSchema             `json:"tool_schemas"`
	Instructions  []InstructionReference   `json:"instructions"`
	Configuration []ConfigurationReference `json:"configuration"`
	Warnings      []Warning                `json:"warnings"`
}

func Validate(receipt Receipt) error {
	if receipt.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported Composition Receipt format version %d", receipt.FormatVersion)
	}
	if !validIdentity(receipt.Preset.ID) || !ValidSHA256Version(receipt.Preset.Version) {
		return errors.New("Composition Receipt preset identity or version is invalid")
	}
	if !validVersion(receipt.EvieVersion) {
		return errors.New("Composition Receipt Evie version must be MAJOR.MINOR.PATCH")
	}
	providerVersions := make(map[string]string, len(receipt.Providers))
	for _, provider := range receipt.Providers {
		if !validIdentity(provider.ID) || strings.Contains(provider.ID, ".") {
			return errors.New("Composition Receipt provider ID must not be empty")
		}
		if _, duplicate := providerVersions[provider.ID]; duplicate {
			return fmt.Errorf("Composition Receipt repeats provider %q", provider.ID)
		}
		if !validVersion(provider.ImplementationVersion) {
			return fmt.Errorf("Composition Receipt provider %q version must be MAJOR.MINOR.PATCH", provider.ID)
		}
		providerVersions[provider.ID] = provider.ImplementationVersion
	}
	capabilities := make(map[string]struct{}, len(receipt.Capabilities))
	for _, capability := range receipt.Capabilities {
		if !validIdentity(capability.ID) || !strings.Contains(capability.ID, ".") ||
			!validIdentity(capability.ProviderID) ||
			providerID(capability.ID) != capability.ProviderID {
			return fmt.Errorf("Composition Receipt Capability %q has an invalid provider identity", capability.ID)
		}
		if _, exists := providerVersions[capability.ProviderID]; !exists {
			return fmt.Errorf("Composition Receipt Capability %q names absent provider %q", capability.ID, capability.ProviderID)
		}
		if _, duplicate := capabilities[capability.ID]; duplicate {
			return fmt.Errorf("Composition Receipt repeats Capability %q", capability.ID)
		}
		capabilities[capability.ID] = struct{}{}
		if !validVersion(capability.ContractVersion) {
			return fmt.Errorf("Composition Receipt Capability %q contract version must be MAJOR.MINOR.PATCH", capability.ID)
		}
		if !ValidSHA256(capability.SchemaSHA256) {
			return fmt.Errorf("Composition Receipt Capability %q schema hash is invalid", capability.ID)
		}
	}
	seenSchemas := make(map[string]struct{}, len(receipt.ToolSchemas))
	for _, schema := range receipt.ToolSchemas {
		if !validIdentity(schema.Name) || !ValidSHA256(schema.SHA256) {
			return fmt.Errorf("Composition Receipt tool schema %q is invalid", schema.Name)
		}
		if _, duplicate := seenSchemas[schema.Name]; duplicate {
			return fmt.Errorf("Composition Receipt repeats tool schema %q", schema.Name)
		}
		seenSchemas[schema.Name] = struct{}{}
	}
	for _, instruction := range receipt.Instructions {
		if !validIdentity(instruction.ID) || !ValidSHA256(instruction.SHA256) {
			return fmt.Errorf("Composition Receipt instruction %q is invalid", instruction.ID)
		}
	}
	for _, reference := range receipt.Configuration {
		if err := ValidateConfigurationReference(reference); err != nil {
			return fmt.Errorf("Composition Receipt: %w", err)
		}
	}
	for _, warning := range receipt.Warnings {
		if !validWarningCode(warning.Code) || !validIdentity(warning.CapabilityID) ||
			!validIdentity(warning.ProviderID) || providerID(warning.CapabilityID) != warning.ProviderID {
			return fmt.Errorf("Composition Receipt warning for Capability %q is invalid", warning.CapabilityID)
		}
	}
	return nil
}

func ValidateConfigurationReference(reference ConfigurationReference) error {
	if reference.Kind != ConfigurationConnection {
		return fmt.Errorf("configuration reference kind %q is not allowed", reference.Kind)
	}
	parsed, err := uuid.Parse(reference.ID)
	if err != nil || parsed.String() != reference.ID {
		return fmt.Errorf("%s configuration reference must be a canonical UUID Connection ID", reference.Kind)
	}
	return nil
}

func ValidSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func ValidSHA256Version(value string) bool {
	return strings.HasPrefix(value, "sha256:") && ValidSHA256(strings.TrimPrefix(value, "sha256:"))
}

func validVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return false
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 128 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func validWarningCode(code WarningCode) bool {
	switch code {
	case WarningProviderNotCompiled, WarningProviderDisabled, WarningProviderStopped,
		WarningProviderWaiting, WarningProviderFailed, WarningCapabilityNotExposed,
		WarningContractIncompatible, WarningProviderContractInvalid:
		return true
	default:
		return false
	}
}

func providerID(capabilityID string) string {
	separator := strings.IndexByte(capabilityID, '.')
	if separator <= 0 {
		return capabilityID
	}
	return capabilityID[:separator]
}

func Clone(receipt Receipt) Receipt {
	receipt.Providers = append([]Provider(nil), receipt.Providers...)
	receipt.Capabilities = append([]Capability(nil), receipt.Capabilities...)
	receipt.ToolSchemas = append([]ToolSchema(nil), receipt.ToolSchemas...)
	receipt.Instructions = append([]InstructionReference(nil), receipt.Instructions...)
	receipt.Configuration = append([]ConfigurationReference(nil), receipt.Configuration...)
	receipt.Warnings = append([]Warning(nil), receipt.Warnings...)
	return receipt
}

func Marshal(receipt Receipt) ([]byte, error) {
	if err := Validate(receipt); err != nil {
		return nil, err
	}
	return json.Marshal(receipt)
}

// Unmarshal rejects unknown fields and trailing content so receipt persistence
// cannot become an accidental credential or extension store.
func Unmarshal(encoded []byte) (Receipt, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var receipt Receipt
	if err := decoder.Decode(&receipt); err != nil {
		return Receipt{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return Receipt{}, errors.New("Composition Receipt has trailing JSON content")
		}
		return Receipt{}, err
	}
	if err := Validate(receipt); err != nil {
		return Receipt{}, err
	}
	return receipt, nil
}

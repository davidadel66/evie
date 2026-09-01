package composition

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReceiptCodecIsClosedToCredentialFields(t *testing.T) {
	receipt := validReceipt()
	encoded, err := Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	withCredential := append([]byte(nil), encoded[:len(encoded)-1]...)
	withCredential = append(withCredential, []byte(`,"access_token":"secret"}`)...)
	if _, err := Unmarshal(withCredential); err == nil || !strings.Contains(err.Error(), `unknown field "access_token"`) {
		t.Fatalf("unknown credential field error = %v", err)
	}
	if bytes.Contains(encoded, []byte("secret")) {
		t.Fatalf("valid encoded receipt contains credential material: %s", encoded)
	}

	receipt.Configuration[0].ID = "raw-token"
	if _, err := Marshal(receipt); err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("raw configuration value error = %v", err)
	}
}

func TestCompatibilityResolutionRequiresStrictlyNewerReplacement(t *testing.T) {
	valid := CompatibilityResolution{
		OriginalProvider:                 Provider{ID: "fixture", ImplementationVersion: "1.2.3"},
		ReplacementImplementationVersion: "1.2.4",
		KernelAPIVersion:                 "1.0.0",
		Capabilities: []CompatibilityCapability{{
			ID: "fixture.echo", ContractVersion: "1.0.0", SchemaSHA256: strings.Repeat("0", 64),
		}},
		ResolvedAt: time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := ValidateCompatibilityResolution(valid); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name        string
		replacement string
		want        string
	}{
		{name: "equal", replacement: "1.2.3", want: "must be newer than original version"},
		{name: "downgrade major", replacement: "0.9.9", want: "must be newer than original version"},
		{name: "downgrade minor", replacement: "1.1.9", want: "must be newer than original version"},
		{name: "downgrade patch", replacement: "1.2.2", want: "must be newer than original version"},
		{name: "malformed", replacement: "1.2", want: "versions must be MAJOR.MINOR.PATCH"},
		{name: "leading zero", replacement: "1.02.4", want: "versions must be MAJOR.MINOR.PATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolution := valid
			resolution.ReplacementImplementationVersion = tc.replacement
			if err := ValidateCompatibilityResolution(resolution); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validation error = %v, want containing %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*CompatibilityResolution)
	}{
		{name: "malformed original", mutate: func(resolution *CompatibilityResolution) {
			resolution.OriginalProvider.ImplementationVersion = "1.2"
		}},
		{name: "malformed Kernel", mutate: func(resolution *CompatibilityResolution) {
			resolution.KernelAPIVersion = "v1.0.0"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolution := valid
			tc.mutate(&resolution)
			if err := ValidateCompatibilityResolution(resolution); err == nil ||
				!strings.Contains(err.Error(), "versions must be MAJOR.MINOR.PATCH") {
				t.Fatalf("validation error = %v, want strict version diagnostic", err)
			}
		})
	}
}

func validReceipt() Receipt {
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	return Receipt{
		FormatVersion: FormatVersion,
		Preset: PresetIdentity{
			ID: "standard", Version: "sha256:" + hash,
		},
		EvieVersion: "1.0.0",
		Providers:   []Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities: []Capability{{
			ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash,
		}},
		ToolSchemas:  []ToolSchema{{Name: "fixture_echo", SHA256: hash}},
		Instructions: []InstructionReference{{ID: "fixture-instructions", SHA256: hash}},
		Configuration: []ConfigurationReference{{
			Kind: ConfigurationConnection, ID: "da73b499-4df4-4a91-bbe8-4fd3a223e634",
		}},
	}
}

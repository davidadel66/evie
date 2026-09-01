package composition

import (
	"bytes"
	"strings"
	"testing"
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

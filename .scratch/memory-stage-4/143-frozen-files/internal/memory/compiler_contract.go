package memory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"unicode/utf8"
)

const CompilerPolicyVersion = "owner-assertions-v1"
const CompilerMaxBytes = 128 * 1024

func CompilerHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Encoding uses the fixed struct field order and compact JSON. Unknown fields
// are rejected when reading configuration, so they cannot disappear from a seal.
func CompilerGenerationIdentity(g CompilerGeneration) (string, []byte, error) {
	if g.Version != "compiler-generation-v1" || g.ProtocolVersion != "ollama-generate-v1" {
		return "", nil, errors.New("unsupported compiler generation or protocol")
	}
	for _, value := range []string{g.ModelArtifact, g.Quantization, g.RuntimeVersion, g.Template, g.Prompt} {
		if strings.TrimSpace(value) == "" {
			return "", nil, errors.New("incomplete compiler generation")
		}
	}
	for _, value := range []string{g.ModelSHA256, g.ModelManifestSHA256, g.TokenizerSHA256, g.TemplateSHA256, g.TokenBoundProofSHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || value != strings.ToLower(value) {
			return "", nil, errors.New("generation requires canonical SHA256 identities")
		}
	}
	if CompilerHash(g.ModelManifest) != g.ModelManifestSHA256 {
		return "", nil, errors.New("model manifest digest mismatch")
	}
	var modelManifest struct {
		Layers []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
		} `json:"layers"`
	}
	if err := json.Unmarshal(g.ModelManifest, &modelManifest); err != nil {
		return "", nil, errors.New("invalid model manifest")
	}
	weights := 0
	for _, layer := range modelManifest.Layers {
		if layer.MediaType == "application/vnd.ollama.image.model" {
			if layer.Digest != "sha256:"+g.ModelSHA256 {
				return "", nil, errors.New("manifest artifact mismatch")
			}
			weights++
		}
	}
	if weights != 1 {
		return "", nil, errors.New("manifest must bind exactly one model artifact")
	}
	if CompilerHash([]byte(g.Template)) != g.TemplateSHA256 {
		return "", nil, errors.New("template digest mismatch")
	}
	if g.EvidencePolicy != CompilerPolicyVersion && g.EvidencePolicy != CompilerClockEvidencePolicy {
		return "", nil, errors.New("unsupported compiler evidence policy")
	}
	for _, policy := range []string{g.SecretPolicy, g.ClosurePolicy, g.WindowPolicy} {
		if policy != CompilerPolicyVersion {
			return "", nil, errors.New("unsupported compiler policy")
		}
	}
	identityPolicy := g.EntityPolicy
	if identityPolicy != CompilerPolicyVersion && identityPolicy != CompilerIdentityPolicyV2 && identityPolicy != CompilerTemporalPolicyV3 {
		return "", nil, errors.New("unsupported compiler identity policy")
	}
	for _, policy := range []string{g.PredicatePolicy, g.ValidationPolicy, g.EquivalencePolicy, g.EffectPolicy} {
		if policy != identityPolicy {
			return "", nil, errors.New("inconsistent compiler interpretation policy")
		}
	}
	if !json.Valid(g.Schema) || bytes.Equal(bytes.TrimSpace(g.Schema), []byte("null")) || len(g.Schema) == 0 {
		return "", nil, errors.New("missing extraction schema")
	}
	if g.Decoding.ContextTokens <= 0 || g.Decoding.ContextTokens > 131072 || g.Decoding.OutputTokens <= 0 || g.Decoding.OutputTokens >= g.Decoding.ContextTokens || g.Decoding.Temperature != 0 || g.TokensPerByte < 1 || g.TokensPerByte > 4 || g.TemplateTokenOverhead < 0 || g.TemplateTokenOverhead > 131072 {
		return "", nil, errors.New("invalid decoding or proven token bound")
	}
	data, err := json.Marshal(g)
	if err != nil {
		return "", nil, err
	}
	if len(data) > CompilerMaxBytes {
		return "", nil, errors.New("generation exceeds input bound")
	}
	return CompilerHash(data), data, nil
}

// DecodeCompilerJSON rejects duplicate object keys, unknown fields, trailing
// values, and oversized documents before any output can influence persistence.
func DecodeCompilerJSON(data []byte, target any) error {
	if err := ValidateCompilerJSON(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	canonical, err := json.Marshal(target)
	if err != nil {
		return err
	}
	var originalShape, decodedShape any
	if err := json.Unmarshal(data, &originalShape); err != nil {
		return err
	}
	if err := json.Unmarshal(canonical, &decodedShape); err != nil {
		return err
	}
	if !reflect.DeepEqual(originalShape, decodedShape) {
		return errors.New("compiler JSON fields must be explicit and canonically named")
	}
	return nil
}

func ValidateCompilerJSON(data []byte) error { return validateCompilerJSON(data, CompilerMaxBytes) }

// Runtime tokenizer metadata has a separate explicit ceiling. It never becomes
// extractor input or a candidate envelope.
const CompilerMetadataMaxBytes = 32 * 1024 * 1024

func ValidateCompilerMetadataJSON(data []byte) error {
	return validateCompilerJSON(data, CompilerMetadataMaxBytes)
}
func validateCompilerJSON(data []byte, limit int) error {
	if len(data) == 0 || len(data) > limit || !utf8.Valid(data) {
		return errors.New("missing or oversized compiler JSON")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := compilerJSONValue(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing compiler JSON")
	}
	return nil
}
func compilerJSONValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("compiler JSON nesting limit")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return errors.New("duplicate compiler JSON key")
			}
			seen[s] = true
			if err := compilerJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := compilerJSONValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}

// CompilerInputBudget uses a pinned conservative byte proof and reserves output
// plus template overhead. It never estimates tokens from characters or truncates.
func CompilerInputBudget(g CompilerGeneration, r CompilerRequest) error {
	if _, _, err := CompilerGenerationIdentity(g); err != nil {
		return err
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	n := len(b) + len(g.Prompt) + len(g.Schema) + len(g.Template)
	if n > CompilerMaxBytes {
		return errors.New("serialized_input_limit")
	}
	bound := int64(n)*int64(g.TokensPerByte) + int64(g.TemplateTokenOverhead) + int64(g.Decoding.OutputTokens)
	if bound > int64(g.Decoding.ContextTokens) {
		return fmt.Errorf("proven_context_limit: %d > %d", bound, g.Decoding.ContextTokens)
	}
	return nil
}

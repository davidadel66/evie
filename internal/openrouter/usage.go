package openrouter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

type jsonMember struct {
	name  string
	value json.RawMessage
}

// UnmarshalJSON keeps provider-specific accounting out of ChatResponse while
// applying the same counter normalization used by streaming responses.
func (r *ChatResponse) UnmarshalJSON(data []byte) error {
	var response struct {
		Choices []Choice `json:"choices"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	usage, _, err := parseProviderUsage(data)
	if err != nil {
		return err
	}
	*r = ChatResponse{Choices: response.Choices, Usage: usage}
	return nil
}

// parseProviderUsage returns the normalized usage and whether this JSON object
// contains a non-null usage occurrence. The latter lets stream assembly replace
// earlier usage with a later empty or invalid occurrence while ignoring null.
func parseProviderUsage(data []byte) (*TokenUsage, bool, error) {
	members, object, err := parseJSONObject(data)
	if err != nil {
		return nil, false, err
	}
	if !object {
		return nil, false, nil
	}

	var occurrences []json.RawMessage
	for _, member := range members {
		if member.name == "usage" {
			occurrences = append(occurrences, member.value)
		}
	}
	if len(occurrences) == 0 {
		return nil, false, nil
	}
	if len(occurrences) > 1 {
		for _, occurrence := range occurrences {
			if !isJSONNull(occurrence) {
				return nil, true, nil
			}
		}
		return nil, false, nil
	}
	if isJSONNull(occurrences[0]) {
		return nil, false, nil
	}

	usage, err := normalizeUsage(occurrences[0])
	return usage, true, err
}

func normalizeUsage(data json.RawMessage) (*TokenUsage, error) {
	members, object, err := parseJSONObject(data)
	if err != nil {
		return nil, err
	}
	if !object {
		return nil, nil
	}

	values := make(map[string][]json.RawMessage)
	for _, member := range members {
		switch member.name {
		case "prompt_tokens", "completion_tokens", "total_tokens":
			values[member.name] = append(values[member.name], member.value)
		case "completion_tokens_details":
			collectDetailCounters(values, member.value, member.name, "reasoning_tokens")
		case "prompt_tokens_details":
			collectDetailCounters(values, member.value, member.name, "cached_tokens", "cache_write_tokens")
		}
	}

	usage := &TokenUsage{
		InputTokens:           normalizedCounter(values["prompt_tokens"]),
		OutputTokens:          normalizedCounter(values["completion_tokens"]),
		TotalTokens:           normalizedCounter(values["total_tokens"]),
		ReasoningOutputTokens: normalizedCounter(values["completion_tokens_details.reasoning_tokens"]),
		CachedInputTokens:     normalizedCounter(values["prompt_tokens_details.cached_tokens"]),
		CacheWriteInputTokens: normalizedCounter(values["prompt_tokens_details.cache_write_tokens"]),
	}
	if usage.InputTokens == nil && usage.OutputTokens == nil && usage.TotalTokens == nil &&
		usage.ReasoningOutputTokens == nil && usage.CachedInputTokens == nil &&
		usage.CacheWriteInputTokens == nil {
		return nil, nil
	}
	return usage, nil
}

func collectDetailCounters(
	values map[string][]json.RawMessage,
	data json.RawMessage,
	container string,
	names ...string,
) {
	members, object, err := parseJSONObject(data)
	if err != nil || !object {
		return
	}
	for _, member := range members {
		for _, name := range names {
			if member.name == name {
				path := container + "." + name
				values[path] = append(values[path], member.value)
			}
		}
	}
}

func normalizedCounter(values []json.RawMessage) *int64 {
	if len(values) != 1 {
		return nil
	}
	text := bytes.TrimSpace(values[0])
	if len(text) == 0 {
		return nil
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return nil
		}
	}
	value, err := strconv.ParseInt(string(text), 10, 64)
	if err != nil {
		return nil
	}
	return &value
}

func parseJSONObject(data []byte) ([]jsonMember, bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	delim, object := token.(json.Delim)
	if !object || delim != '{' {
		return nil, false, nil
	}

	var members []jsonMember
	for decoder.More() {
		nameToken, err := decoder.Token()
		if err != nil {
			return nil, false, err
		}
		name, ok := nameToken.(string)
		if !ok {
			return nil, false, fmt.Errorf("JSON object member name has type %T", nameToken)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false, err
		}
		members = append(members, jsonMember{name: name, value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return nil, false, err
		}
		return nil, false, fmt.Errorf("unexpected trailing JSON token %v", token)
	}
	return members, true, nil
}

func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

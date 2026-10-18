package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

// turnDecodes remembers, for one turn, what decoding a stored event yields.
// Stored events never change, so each of a turn's requests decodes only the
// events committed since the one before. A nil *turnDecodes decodes every
// time, which is what callers outside a turn get.
type turnDecodes struct {
	snapshots   map[memory.EventID]snapshotDecode
	calibration map[memory.EventID]calibrationDecode
	digests     map[memory.EventID]string
}

type snapshotDecode struct {
	payload memory.ContextSnapshotPayload
	err     error
}

// calibrationDecode is what bytesPerToken reads from one event: a context
// snapshot's payload, or an assistant message's reported input tokens. ok is
// false when the event yields no sample.
type calibrationDecode struct {
	snapshot    memory.ContextSnapshotPayload
	inputTokens int64
	ok          bool
}

func newTurnDecodes() *turnDecodes {
	return &turnDecodes{
		snapshots:   make(map[memory.EventID]snapshotDecode),
		calibration: make(map[memory.EventID]calibrationDecode),
		digests:     make(map[memory.EventID]string),
	}
}

// snapshot validates a context snapshot event and decodes its payload.
func (d *turnDecodes) snapshot(event memory.Event) (memory.ContextSnapshotPayload, error) {
	if d != nil {
		if cached, ok := d.snapshots[event.ID]; ok {
			return cached.payload, cached.err
		}
	}
	var payload memory.ContextSnapshotPayload
	err := validateSnapshotEvent(event)
	if err == nil {
		if decodeErr := json.Unmarshal(event.Payload, &payload); decodeErr != nil {
			err = fmt.Errorf("decode context snapshot event %q: %w", event.ID, decodeErr)
		}
	}
	if d != nil {
		d.snapshots[event.ID] = snapshotDecode{payload: payload, err: err}
	}
	return payload, err
}

// calibrationSnapshot decodes a context snapshot's payload leniently, as
// calibration always has.
func (d *turnDecodes) calibrationSnapshot(event memory.Event) (memory.ContextSnapshotPayload, bool) {
	decoded := d.calibrationDecode(event, func() calibrationDecode {
		var snapshot memory.ContextSnapshotPayload
		if json.Unmarshal(event.Payload, &snapshot) != nil {
			return calibrationDecode{}
		}
		return calibrationDecode{snapshot: snapshot, ok: true}
	})
	return decoded.snapshot, decoded.ok
}

// assistantInputTokens is an assistant message's reported positive input
// token count.
func (d *turnDecodes) assistantInputTokens(event memory.Event) (int64, bool) {
	decoded := d.calibrationDecode(event, func() calibrationDecode {
		var payload memory.AssistantMessagePayload
		if json.Unmarshal(event.Payload, &payload) != nil || payload.Usage == nil ||
			payload.Usage.InputTokens == nil || *payload.Usage.InputTokens <= 0 {
			return calibrationDecode{}
		}
		return calibrationDecode{inputTokens: *payload.Usage.InputTokens, ok: true}
	})
	return decoded.inputTokens, decoded.ok
}

func (d *turnDecodes) calibrationDecode(event memory.Event, decode func() calibrationDecode) calibrationDecode {
	if d == nil {
		return decode()
	}
	if cached, ok := d.calibration[event.ID]; ok {
		return cached
	}
	decoded := decode()
	d.calibration[event.ID] = decoded
	return decoded
}

// contentDigest is the hex SHA-256 of an event's content, which placeholders
// record for the tool results they replace.
func (d *turnDecodes) contentDigest(event memory.Event) string {
	if d != nil {
		if digest, ok := d.digests[event.ID]; ok {
			return digest
		}
	}
	sum := sha256.Sum256([]byte(event.Content))
	digest := hex.EncodeToString(sum[:])
	if d != nil {
		d.digests[event.ID] = digest
	}
	return digest
}

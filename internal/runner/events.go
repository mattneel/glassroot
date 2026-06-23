package runner

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/mattneel/glassroot/internal/model"
)

const eventIDDomain = "glassroot.dev/observation-event-id/v1\x00"

func eventID(planDigest model.Digest, runID string, sequence uint64) string {
	h := sha256.New()
	_, _ = h.Write([]byte(eventIDDomain))
	writeLengthPrefixed(h, []byte(planDigest))
	writeLengthPrefixed(h, []byte(runID))
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], sequence)
	_, _ = h.Write(n[:])
	return "evt-" + hex.EncodeToString(h.Sum(nil))
}

type byteWriter interface{ Write([]byte) (int, error) }

func writeLengthPrefixed(w byteWriter, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = w.Write(n[:])
	_, _ = w.Write(b)
}

func envelopeEvent(attempt AttemptRequest, sequence uint64, draft EventDraft) model.ObservationEvent {
	draft = cloneEventDraft(draft)
	observedAt := draft.ObservedAt.UTC().Round(0)
	if observedAt.IsZero() {
		observedAt = deterministicAttemptTime(attempt, 0)
	}
	return model.ObservationEvent{
		SchemaVersion:   model.SchemaVersionObservationEventV1Alpha1,
		ID:              eventID(attempt.PlanDigest, attempt.RunID, sequence),
		RunID:           attempt.RunID,
		Revision:        attempt.Revision,
		ScenarioID:      attempt.ScenarioID,
		Repetition:      attempt.Repetition,
		SequenceNumber:  int64(sequence),
		ObservedAt:      observedAt,
		Source:          draft.Source,
		Kind:            draft.Kind,
		Process:         draft.Process,
		Filesystem:      draft.Filesystem,
		Network:         draft.Network,
		Artifact:        draft.Artifact,
		Scenario:        draft.Scenario,
		ObserverWarning: draft.ObserverWarning,
		ResourceLimit:   draft.ResourceLimit,
	}
}

func deterministicAttemptTime(attempt AttemptRequest, offsetMillis int64) time.Time {
	base := attempt.PlanCreatedAt.UTC().Round(0)
	if attempt.GlobalOrdinal > 0 {
		base = base.Add(time.Duration(attempt.GlobalOrdinal-1) * time.Second)
	}
	return base.Add(time.Duration(offsetMillis) * time.Millisecond)
}

func eventJSONSize(event model.ObservationEvent) (int, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

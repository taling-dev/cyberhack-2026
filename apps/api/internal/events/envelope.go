// Package events defines the standardized event envelope used for all SimaOps
// domain events flowing through the outbox → NATS → SSE pipeline.
//
// Envelopes are stored as the JSON payload of an outbox_events row (in
// outbox_events.payload_json) and are passed through unchanged by the outbox
// publisher when forwarding to NATS. Downstream consumers (AI worker, SSE
// handler) parse the envelope to extract the inner payload plus the routing/
// auth metadata required for owner-scoped fan-out.
//
// Schema:
//
//	{
//	  "event_id":      "uuid",                 // unique per event
//	  "event_type":    "qc.job.created",       // = NATS subject
//	  "occurred_at":   "2026-05-27T04:10:00Z", // RFC3339 UTC
//	  "actor_id":      "kc-user-uuid",         // who triggered the event
//	  "owner_user_id": "kc-user-uuid",         // resource owner (created_by)
//	  "resource_id":   "lot-uuid|qc-job-uuid", // primary entity referenced
//	  "payload":       { ... domain-specific ... }
//	}
//
// `owner_user_id` is what the SSE handler uses to enforce per-OPERATOR owner
// filtering — operators only receive events whose owner_user_id == their JWT
// sub. For events about a QC job, the owner is the lot's created_by, not the
// QC requester (so an operator who uploaded a QC image for someone else's lot
// would not see that lot's events).
package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Envelope is the canonical wrapper for every domain event published to NATS.
type Envelope struct {
	EventID     string          `json:"event_id"`
	EventType   string          `json:"event_type"`
	OccurredAt  time.Time       `json:"occurred_at"`
	ActorID     string          `json:"actor_id"`
	OwnerUserID string          `json:"owner_user_id"`
	ResourceID  string          `json:"resource_id"`
	Payload     json.RawMessage `json:"payload"`
}

// NewEnvelope constructs a new envelope and serializes it to JSON ready for
// storage in outbox_events.payload_json or direct publish to NATS.
//
// payload may be any JSON-serializable value or nil. nil/empty payloads are
// preserved as `null` in the envelope so downstream consumers can distinguish
// "no payload" from "empty payload".
func NewEnvelope(eventType, actorID, ownerID, resourceID string, payload any) ([]byte, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		raw = b
	} else {
		raw = []byte("null")
	}

	env := Envelope{
		EventID:     uuid.NewString(),
		EventType:   eventType,
		OccurredAt:  time.Now().UTC(),
		ActorID:     actorID,
		OwnerUserID: ownerID,
		ResourceID:  resourceID,
		Payload:     raw,
	}
	return json.Marshal(env)
}

// ParseEnvelope decodes a JSON envelope. It returns an error if the input is
// not valid JSON or if event_type is missing — those are the only fields the
// SSE filter needs in order to make a routing decision.
func ParseEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

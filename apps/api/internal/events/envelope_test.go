package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewEnvelope_BasicShape(t *testing.T) {
	type lotPayload struct {
		LotNumber string `json:"lot_number"`
	}
	raw, err := NewEnvelope(
		"lot.created",
		"actor-uuid",
		"owner-uuid",
		"lot-uuid",
		lotPayload{LotNumber: "LOT-2026-05-27-A1B2"},
	)
	if err != nil {
		t.Fatalf("NewEnvelope error: %v", err)
	}

	env, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatalf("ParseEnvelope error: %v", err)
	}
	if env.EventID == "" {
		t.Error("expected event_id to be populated")
	}
	if env.EventType != "lot.created" {
		t.Errorf("event_type = %q, want lot.created", env.EventType)
	}
	if env.ActorID != "actor-uuid" {
		t.Errorf("actor_id = %q", env.ActorID)
	}
	if env.OwnerUserID != "owner-uuid" {
		t.Errorf("owner_user_id = %q", env.OwnerUserID)
	}
	if env.ResourceID != "lot-uuid" {
		t.Errorf("resource_id = %q", env.ResourceID)
	}
	// occurred_at should be very recent (within last 5s)
	if d := time.Since(env.OccurredAt); d > 5*time.Second || d < 0 {
		t.Errorf("occurred_at not recent: %v", env.OccurredAt)
	}
	if !strings.Contains(string(env.Payload), "LOT-2026-05-27-A1B2") {
		t.Errorf("payload missing data: %s", env.Payload)
	}
}

func TestNewEnvelope_NilPayload(t *testing.T) {
	raw, err := NewEnvelope("audit.log_created", "a", "o", "r", nil)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	env, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if string(env.Payload) != "null" {
		t.Errorf("nil payload should serialize to null, got: %s", env.Payload)
	}
}

func TestNewEnvelope_MapPayload(t *testing.T) {
	raw, err := NewEnvelope(
		"qc.job.created",
		"actor",
		"owner",
		"qc-uuid",
		map[string]string{
			"qc_job_id":        "qc-uuid",
			"lot_id":           "lot-uuid",
			"image_object_key": "lot-uuid/img.jpg",
		},
	)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	env, err := ParseEnvelope(raw)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	var inner map[string]string
	if err := json.Unmarshal(env.Payload, &inner); err != nil {
		t.Fatalf("inner unmarshal: %v", err)
	}
	if inner["qc_job_id"] != "qc-uuid" {
		t.Errorf("qc_job_id = %q", inner["qc_job_id"])
	}
	if inner["lot_id"] != "lot-uuid" {
		t.Errorf("lot_id = %q", inner["lot_id"])
	}
}

func TestParseEnvelope_InvalidJSON(t *testing.T) {
	if _, err := ParseEnvelope([]byte("not json")); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestNewEnvelope_UniqueEventIDs(t *testing.T) {
	r1, _ := NewEnvelope("x", "a", "o", "r", nil)
	r2, _ := NewEnvelope("x", "a", "o", "r", nil)
	e1, _ := ParseEnvelope(r1)
	e2, _ := ParseEnvelope(r2)
	if e1.EventID == e2.EventID {
		t.Errorf("event IDs collide: %s", e1.EventID)
	}
}

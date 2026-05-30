package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

// slotAcceptsDrum mirrors the hazard/drum decision in RecommendSlot so the
// rule is unit-tested without a DB: drum_compatibility is the physical
// constraint (required); hazard_allowed is segregation (enforced only when
// non-empty). hazardClass is the proto-enum string stored on the lot.
func slotAcceptsDrum(hazardAllowed, drumCompat json.RawMessage, hazardClass string) bool {
	if hazardClass == "" || hazardClass == "HAZARD_CLASS_NONE" {
		return true
	}
	drum := strings.TrimPrefix(hazardClass, "HAZARD_CLASS_")
	if !jsonArrayContains(drumCompat, drum) {
		return false
	}
	if !jsonArrayIsEmpty(hazardAllowed) && !jsonArrayContains(hazardAllowed, drum) {
		return false
	}
	return true
}

func TestSlotAcceptsDrum(t *testing.T) {
	zoneAB_hazard := json.RawMessage(`[]`)
	zoneAB_drum := json.RawMessage(`["IBC","IPPC"]`)
	zoneC_hazard := json.RawMessage(`["IBC"]`)
	zoneC_drum := json.RawMessage(`["IBC"]`)

	cases := []struct {
		name                  string
		hazardAllowed, drum   json.RawMessage
		hazardClass           string
		want                  bool
	}{
		{"no hazard class accepted anywhere", zoneAB_hazard, zoneAB_drum, "HAZARD_CLASS_NONE", true},
		{"IBC into Zone A/B (empty hazard list = no segregation)", zoneAB_hazard, zoneAB_drum, "HAZARD_CLASS_IBC", true},
		{"IPPC into Zone A/B", zoneAB_hazard, zoneAB_drum, "HAZARD_CLASS_IPPC", true},
		{"IBC into Zone C (drum+hazard both allow)", zoneC_hazard, zoneC_drum, "HAZARD_CLASS_IBC", true},
		{"IPPC into Zone C rejected (drum not compatible)", zoneC_hazard, zoneC_drum, "HAZARD_CLASS_IPPC", false},
		{"empty hazard string treated as none", zoneC_hazard, zoneC_drum, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slotAcceptsDrum(c.hazardAllowed, c.drum, c.hazardClass); got != c.want {
				t.Errorf("slotAcceptsDrum(%s) = %v, want %v", c.hazardClass, got, c.want)
			}
		})
	}
}

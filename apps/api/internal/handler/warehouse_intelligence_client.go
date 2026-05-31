package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	whv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1"
)

// JSON contract with apps/warehouse-intelligence (POST /slotting/recommend).
type wiStorageRequirement struct {
	TemperatureRange string `json:"temperature_range"`
	HazardClass      string `json:"hazard_class,omitempty"`
}
type wiLocation struct {
	ID                string   `json:"id"`
	Code              string   `json:"code"`
	Zone              string   `json:"zone"`
	TemperatureMin    float64  `json:"temperature_min"`
	TemperatureMax    float64  `json:"temperature_max"`
	Capacity          int32    `json:"capacity"`
	HazardAllowed     []string `json:"hazard_allowed"`
	DrumCompatibility []string `json:"drum_compatibility"`
}
type wiRecommendRequest struct {
	StorageRequirement wiStorageRequirement `json:"storage_requirement"`
	Locations          []wiLocation         `json:"locations"`
}
type wiRecommendation struct {
	LocationID string  `json:"location_id"`
	Reason     string  `json:"reason"`
	Score      float64 `json:"score"`
}
type wiRecommendResponse struct {
	Recommendations []wiRecommendation `json:"recommendations"`
}

var wiHTTPClient = &http.Client{Timeout: 3 * time.Second}

// recommendSlotViaIntelligence delegates the decision to the warehouse
// intelligence service. Returns (recs, true) on success; (nil, false) when the
// service is unconfigured or any step fails, so the caller falls back to the
// inline rule logic.
func (s *WarehouseService) recommendSlotViaIntelligence(ctx context.Context, lot db.Lot, available []db.WarehouseLocation) ([]*whv1.SlotRecommendation, bool) {
	if s.intelligenceURL == "" {
		return nil, false
	}

	var sr struct {
		TemperatureRange string  `json:"temperature_range"`
		HazardClass      *string `json:"hazard_class"`
	}
	json.Unmarshal(lot.StorageRequirement, &sr)

	reqBody := wiRecommendRequest{
		StorageRequirement: wiStorageRequirement{TemperatureRange: sr.TemperatureRange},
	}
	if sr.HazardClass != nil {
		reqBody.StorageRequirement.HazardClass = *sr.HazardClass
	}
	// Map db rows to the service contract, decoding the JSON arrays once here.
	byID := make(map[string]db.WarehouseLocation, len(available))
	for _, loc := range available {
		var hazards, drums []string
		json.Unmarshal(loc.HazardAllowed, &hazards)
		json.Unmarshal(loc.DrumCompatibility, &drums)
		reqBody.Locations = append(reqBody.Locations, wiLocation{
			ID:                loc.ID,
			Code:              loc.Code,
			Zone:              loc.Zone,
			TemperatureMin:    parseFloat(loc.TemperatureMin),
			TemperatureMax:    parseFloat(loc.TemperatureMax),
			Capacity:          loc.Capacity,
			HazardAllowed:     hazards,
			DrumCompatibility: drums,
		})
		byID[loc.ID] = loc
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, false
	}
	url := strings.TrimRight(s.intelligenceURL, "/") + "/slotting/recommend"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, false
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := wiHTTPClient.Do(httpReq)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}

	var out wiRecommendResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false
	}

	recs := make([]*whv1.SlotRecommendation, 0, len(out.Recommendations))
	for _, r := range out.Recommendations {
		loc, ok := byID[r.LocationID]
		if !ok {
			continue
		}
		recs = append(recs, &whv1.SlotRecommendation{
			Location: dbLocationToProto(loc),
			Reason:   r.Reason,
			Score:    r.Score,
		})
	}
	return recs, true
}

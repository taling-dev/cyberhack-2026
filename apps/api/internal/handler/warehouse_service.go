package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	whv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1/warehousev1connect"
)

var _ warehousev1connect.WarehouseServiceHandler = (*WarehouseService)(nil)

type WarehouseService struct {
	q *db.Queries
}

func NewWarehouseService(q *db.Queries) *WarehouseService {
	return &WarehouseService{q: q}
}

func (s *WarehouseService) ListLocations(ctx context.Context, req *connect.Request[whv1.ListLocationsRequest]) (*connect.Response[whv1.ListLocationsResponse], error) {
	locations, err := s.q.ListWarehouseLocations(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*whv1.WarehouseLocation, len(locations))
	for i, l := range locations {
		protos[i] = dbLocationToProto(l)
	}
	return connect.NewResponse(&whv1.ListLocationsResponse{Locations: protos}), nil
}

func (s *WarehouseService) RecommendSlot(ctx context.Context, req *connect.Request[whv1.RecommendSlotRequest]) (*connect.Response[whv1.RecommendSlotResponse], error) {
	lot, err := s.q.GetLot(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}

	// Parse storage requirement
	var sr struct {
		TemperatureRange string  `json:"temperature_range"`
		HazardClass      *string `json:"hazard_class"`
	}
	json.Unmarshal(lot.StorageRequirement, &sr)

	// Get temp bounds from range
	minTemp, maxTemp := tempBounds(sr.TemperatureRange)

	// Get available locations
	available, err := s.q.ListAvailableLocations(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var recs []*whv1.SlotRecommendation
	for _, loc := range available {
		locMin := parseFloat(loc.TemperatureMin)
		locMax := parseFloat(loc.TemperatureMax)

		// Filter: location temp range must contain the lot's required range
		if locMin > minTemp || locMax < maxTemp {
			continue
		}

		// Filter: hazard class compatibility
		if sr.HazardClass != nil && *sr.HazardClass != "" && *sr.HazardClass != "HAZARD_CLASS_NONE" {
			if !jsonArrayContains(loc.HazardAllowed, *sr.HazardClass) &&
				!jsonArrayContains(loc.DrumCompatibility, *sr.HazardClass) {
				continue
			}
		}

		score := float64(loc.Capacity) // simple: prefer higher capacity
		reason := fmt.Sprintf("matches %s (%.0f to %.0f °C)", sr.TemperatureRange, locMin, locMax)
		if sr.HazardClass != nil && *sr.HazardClass != "" && *sr.HazardClass != "HAZARD_CLASS_NONE" {
			reason += fmt.Sprintf(" + %s drum", *sr.HazardClass)
		}

		recs = append(recs, &whv1.SlotRecommendation{
			Location: dbLocationToProto(loc),
			Reason:   reason,
			Score:    score,
		})
	}

	return connect.NewResponse(&whv1.RecommendSlotResponse{Recommendations: recs}), nil
}

func (s *WarehouseService) AssignSlot(ctx context.Context, req *connect.Request[whv1.AssignSlotRequest]) (*connect.Response[whv1.AssignSlotResponse], error) {
	msg := req.Msg
	if msg.LotId == "" || msg.LocationId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and location_id required"))
	}

	// Validate lot is QC_APPROVED
	lot, err := s.q.GetLot(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	if lot.Status != db.LotsStatusQCAPPROVED {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("lot must be QC_APPROVED (current: %s)", lot.Status))
	}

	// Atomic capacity decrement: only succeeds if capacity > 0
	// Returns rows affected = 1 if successful, 0 if no capacity left
	rowsAffected, err := s.q.DecrementLocationCapacityAtomic(ctx, msg.LocationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("capacity check: %w", err))
	}
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no capacity available at this location"))
	}

	// Fetch location for response
	loc, err := s.q.GetWarehouseLocation(ctx, msg.LocationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("location not found"))
	}

	assignedBy := userFromCtx(ctx)
	assignmentID := uuid.NewString()
	err = s.q.CreateWarehouseAssignment(ctx, db.CreateWarehouseAssignmentParams{
		ID:         assignmentID,
		LotID:      msg.LotId,
		LocationID: msg.LocationId,
		AssignedBy: assignedBy,
	})
	if err != nil {
		// Rollback capacity (best effort)
		_, _ = s.q.IncrementLocationCapacity(ctx, msg.LocationId)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create assignment: %w", err))
	}

	// Advance lot: WAREHOUSE_ASSIGNED → READY_FOR_PRODUCTION
	if err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusREADYFORPRODUCTION, ID: msg.LotId}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	return connect.NewResponse(&whv1.AssignSlotResponse{
		Assignment: &whv1.WarehouseAssignment{
			Id:           assignmentID,
			LotId:        msg.LotId,
			LocationId:   msg.LocationId,
			LocationCode: loc.Code,
			AssignedBy:   assignedBy,
			AssignedAt:   timestamppb.Now(),
			Status:       whv1.AssignmentStatus_ASSIGNMENT_STATUS_ACTIVE,
		},
	}), nil
}

func (s *WarehouseService) GetWarehouseAssignments(ctx context.Context, req *connect.Request[whv1.GetWarehouseAssignmentsRequest]) (*connect.Response[whv1.GetWarehouseAssignmentsResponse], error) {
	assignments, err := s.q.ListWarehouseAssignments(ctx, db.ListWarehouseAssignmentsParams{Limit: 50, Offset: 0})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	protos := make([]*whv1.WarehouseAssignment, len(assignments))
	for i, a := range assignments {
		protos[i] = &whv1.WarehouseAssignment{
			Id:         a.ID,
			LotId:      a.LotID,
			LocationId: a.LocationID,
			AssignedBy: a.AssignedBy,
			AssignedAt: timestamppb.New(a.AssignedAt),
		}
	}
	return connect.NewResponse(&whv1.GetWarehouseAssignmentsResponse{Assignments: protos}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────

func dbLocationToProto(l db.WarehouseLocation) *whv1.WarehouseLocation {
	loc := &whv1.WarehouseLocation{
		Id:              l.ID,
		Code:            l.Code,
		Zone:            l.Zone,
		TemperatureMin:  parseFloat(l.TemperatureMin),
		TemperatureMax:  parseFloat(l.TemperatureMax),
		Capacity:        l.Capacity,
		CurrentStatus:   locationStatusFromDB(string(l.CurrentStatus)),
	}

	// Parse hazard_allowed JSON array
	var hazards []string
	if json.Unmarshal(l.HazardAllowed, &hazards) == nil {
		for _, h := range hazards {
			loc.HazardAllowed = append(loc.HazardAllowed, hazardClassFromString("HAZARD_CLASS_"+h))
		}
	}

	// Parse drum_compatibility JSON array
	var drums []string
	if json.Unmarshal(l.DrumCompatibility, &drums) == nil {
		loc.DrumCompatibility = drums
	}

	return loc
}

func locationStatusFromDB(s string) whv1.LocationStatus {
	switch s {
	case "AVAILABLE":
		return whv1.LocationStatus_LOCATION_STATUS_AVAILABLE
	case "OCCUPIED":
		return whv1.LocationStatus_LOCATION_STATUS_OCCUPIED
	case "MAINTENANCE":
		return whv1.LocationStatus_LOCATION_STATUS_MAINTENANCE
	default:
		return whv1.LocationStatus_LOCATION_STATUS_UNSPECIFIED
	}
}

func tempBounds(rangeStr string) (min, max float64) {
	switch rangeStr {
	case "TEMPERATURE_RANGE_AMBIENT":
		return 15, 25
	case "TEMPERATURE_RANGE_COLD":
		return 2, 8
	case "TEMPERATURE_RANGE_DEEP_FREEZE":
		return -20, -4
	default:
		return 15, 25
	}
}

func jsonArrayContains(raw json.RawMessage, val string) bool {
	var arr []string
	if json.Unmarshal(raw, &arr) != nil {
		return false
	}
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

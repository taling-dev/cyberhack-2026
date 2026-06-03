package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	whv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1/warehousev1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
)

var _ warehousev1connect.WarehouseServiceHandler = (*WarehouseService)(nil)

type WarehouseService struct {
	q               *db.Queries
	dbx             *sql.DB
	intelligenceURL string
}

func NewWarehouseService(q *db.Queries, dbx *sql.DB) *WarehouseService {
	return &WarehouseService{q: q, dbx: dbx, intelligenceURL: os.Getenv("WAREHOUSE_INTELLIGENCE_URL")}
}

// autoAssign assigns the top-ranked recommended slot to a freshly QC_APPROVED
// lot. Best-effort: any error (no slot, capacity race) leaves the lot
// QC_APPROVED for manual assignment from the warehouse queue.
func (s *WarehouseService) autoAssign(ctx context.Context, lotID string) {
	recResp, err := s.RecommendSlot(ctx, connect.NewRequest(&whv1.RecommendSlotRequest{LotId: lotID}))
	if err != nil || len(recResp.Msg.Recommendations) == 0 {
		return
	}
	top := recResp.Msg.Recommendations[0]
	if top.Location == nil {
		return
	}
	// Auto-assign with AUTO decision type - visibility is key
	_, _ = s.AssignSlotWithDecisionType(ctx, connect.NewRequest(&whv1.AssignSlotRequest{
		LotId:      lotID,
		LocationId: top.Location.Id,
	}), whv1.DecisionType_DECISION_TYPE_AUTO, top.Reason)
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
	available, err := s.q.ListAvailableLocations(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Delegate the decision to the warehouse-intelligence service when
	// configured; on any error fall back to the inline rule logic so slot
	// recommendation never hard-depends on the Python service being up.
	if recs, ok := s.recommendSlotViaIntelligence(ctx, lot, available); ok {
		return connect.NewResponse(&whv1.RecommendSlotResponse{Recommendations: recs}), nil
	}
	return connect.NewResponse(&whv1.RecommendSlotResponse{
		Recommendations: recommendSlotInline(lot, available),
	}), nil
}

// recommendSlotInline is the original rule-based recommender, kept as the
// fallback when the intelligence service is unset or unreachable.
func recommendSlotInline(lot db.Lot, available []db.WarehouseLocation) []*whv1.SlotRecommendation {
	var sr struct {
		TemperatureRange string  `json:"temperature_range"`
		HazardClass      *string `json:"hazard_class"`
	}
	json.Unmarshal(lot.StorageRequirement, &sr)

	minTemp, maxTemp := tempBounds(sr.TemperatureRange)

	var recs []*whv1.SlotRecommendation
	for _, loc := range available {
		locMin := parseFloat(loc.TemperatureMin)
		locMax := parseFloat(loc.TemperatureMax)

		// Filter: location temp range must contain the lot's required range
		if locMin > minTemp || locMax < maxTemp {
			continue
		}

		// Filter: drum + hazard compatibility.
		if sr.HazardClass != nil && *sr.HazardClass != "" && *sr.HazardClass != "HAZARD_CLASS_NONE" {
			drum := strings.TrimPrefix(*sr.HazardClass, "HAZARD_CLASS_")
			if !jsonArrayContains(loc.DrumCompatibility, drum) {
				continue // slot can't physically hold this drum type
			}
			if !jsonArrayIsEmpty(loc.HazardAllowed) && !jsonArrayContains(loc.HazardAllowed, drum) {
				continue // zone segregation rejects this hazard class
			}
		}

		score := float64(loc.Capacity) // simple: prefer higher capacity
		reason := fmt.Sprintf("matches %s (%.0f to %.0f °C)", sr.TemperatureRange, locMin, locMax)
		if sr.HazardClass != nil && *sr.HazardClass != "" && *sr.HazardClass != "HAZARD_CLASS_NONE" {
			reason += fmt.Sprintf(" + %s drum", strings.TrimPrefix(*sr.HazardClass, "HAZARD_CLASS_"))
		}

		recs = append(recs, &whv1.SlotRecommendation{
			Location:         dbLocationToProto(loc),
			Reason:           reason,
			Score:            score,
			IsAutoAssignable: true, // All recommendations from inline are auto-assignable
		})
	}

	return recs
}

func (s *WarehouseService) AssignSlot(ctx context.Context, req *connect.Request[whv1.AssignSlotRequest]) (*connect.Response[whv1.AssignSlotResponse], error) {
	// Default to MANUAL for explicit user assignments
	return s.AssignSlotWithDecisionType(ctx, req, whv1.DecisionType_DECISION_TYPE_MANUAL, "")
}

func (s *WarehouseService) AssignSlotWithDecisionType(ctx context.Context, req *connect.Request[whv1.AssignSlotRequest], decisionType whv1.DecisionType, reason string) (*connect.Response[whv1.AssignSlotResponse], error) {
	msg := req.Msg
	if msg.LotId == "" || msg.LocationId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and location_id required"))
	}

	assignedBy := userFromCtx(ctx)
	assignmentID := uuid.NewString()
	decisionID := uuid.NewString()

	// Single transaction wrapping every mutation:
	//   1. Lock the lot row + validate status (FOR UPDATE — prevents concurrent
	//      AssignSlot calls from both seeing QC_APPROVED and racing).
	//   2. Atomic capacity decrement — `UPDATE … capacity = capacity - 1
	//      WHERE id = ? AND capacity > 0` returning rowsAffected=1 only if
	//      we won the race for the last unit.
	//   3. Read the location for the response.
	//   4. Insert the assignment row with decision_type.
	//   5. Insert slot_decision audit row.
	//   6. Advance the lot to READY_FOR_PRODUCTION.
	//   7. Append both outbox events (warehouse.slot_assigned + lot.status_changed).
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// 1. Lot must be QC_APPROVED. Reading FOR UPDATE keeps the row locked so
	//    a concurrent transition can't race us.
	lot, err := qtx.GetLotForUpdate(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	if lot.Status != db.LotsStatusQCAPPROVED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("lot must be QC_APPROVED (current: %s)", lot.Status))
	}

	// 2. Atomic capacity decrement INSIDE the tx. On rollback, this is
	//    automatically undone — no separate increment-on-failure needed.
	rowsAffected, err := qtx.DecrementLocationCapacityAtomic(ctx, msg.LocationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("capacity check: %w", err))
	}
	if rowsAffected == 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no capacity available at this location"))
	}

	// 3. Fetch the (now-decremented) location for the response payload.
	loc, err := qtx.GetWarehouseLocation(ctx, msg.LocationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("location not found"))
	}

	// 3b. Flip the slot to OCCUPIED once its last unit is taken so
	//     ListAvailableLocations (which filters current_status='AVAILABLE')
	//     stops recommending a full slot. Capacity > 0 stays AVAILABLE.
	if loc.Capacity == 0 {
		if err := qtx.UpdateLocationStatus(ctx, db.UpdateLocationStatusParams{
			CurrentStatus: db.WarehouseLocationsCurrentStatusOCCUPIED,
			ID:            msg.LocationId,
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update location status: %w", err))
		}
	}

	// 4. Insert the assignment row with decision_type.
	dt := decisionTypeToDB(decisionType)
	if err := qtx.CreateWarehouseAssignmentWithDecisionType(ctx, db.CreateWarehouseAssignmentWithDecisionTypeParams{
		ID:           assignmentID,
		LotID:        msg.LotId,
		LocationID:   msg.LocationId,
		AssignedBy:   assignedBy,
		DecisionType: dt,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create assignment: %w", err))
	}

	// 5. Insert slot_decision audit row for visibility.
	if err := qtx.CreateSlotDecision(ctx, db.CreateSlotDecisionParams{
		ID:            decisionID,
		LotID:         msg.LotId,
		LocationID:    msg.LocationId,
		DecisionType:  dt,
		Reason:        sql.NullString{String: reason, Valid: reason != ""},
		ActorID:       assignedBy,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create slot decision: %w", err))
	}

	// 6. Advance the lot.
	if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatusREADYFORPRODUCTION,
		ID:     msg.LotId,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	// 7a. Emit warehouse.slot_assigned for SSE fan-out. Owner is the lot
	//     creator so operators get a "your lot was slotted" toast.
	whEnvelope, err := events.NewEnvelope(
		"warehouse.slot_assigned",
		assignedBy,
		lot.CreatedBy,
		msg.LotId,
		map[string]any{
			"assignment_id":   assignmentID,
			"lot_id":          msg.LotId,
			"lot_number":      lot.LotNumber,
			"lot_created_by":  lot.CreatedBy,
			"location_id":     msg.LocationId,
			"location_code":   loc.Code,
			"assigned_by":     assignedBy,
			"decision_type":   decisionTypeToDB(decisionType),
			"reason":          reason,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "warehouse.slot_assigned",
		PayloadJson: whEnvelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	// 7b. Emit lot.status_changed so subscribers tracking only lot.* see the
	//     final transition to READY_FOR_PRODUCTION.
	statusEnvelope, err := events.NewEnvelope(
		"lot.status_changed",
		assignedBy,
		lot.CreatedBy,
		msg.LotId,
		map[string]any{
			"lot_id":     msg.LotId,
			"lot_number": lot.LotNumber,
			"from":       "QC_APPROVED",
			"to":         "READY_FOR_PRODUCTION",
			"reason":     "warehouse-assigned",
			"created_by": lot.CreatedBy,
			"actor_id":   assignedBy,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build status envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "lot.status_changed",
		PayloadJson: statusEnvelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create lot status outbox event: %w", err))
	}

	// 7c. Emit lot.ready_for_production — the dedicated production-handoff
	//     event. Unlike the generic lot.status_changed firehose, this carries
	//     the full lot payload a downstream dispatch / PPIC consumer needs to
	//     act, and is a distinct subject that can be granted via the SSE role
	//     filter without exposing every status change. This is the clean
	//     integration seam that closes the Integrated Operations System loop
	//     from warehouse → production handoff → dispatch.
	if err := emitLotReadyForProduction(ctx, qtx, assignedBy, lot, loc.Code); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	middleware.IncWarehouseAssignment()

	return connect.NewResponse(&whv1.AssignSlotResponse{
		Assignment: &whv1.WarehouseAssignment{
			Id:           assignmentID,
			LotId:        msg.LotId,
			LocationId:   msg.LocationId,
			LocationCode: loc.Code,
			AssignedBy:   assignedBy,
			AssignedAt:   timestamppb.Now(),
			Status:       whv1.AssignmentStatus_ASSIGNMENT_STATUS_ACTIVE,
			DecisionType: decisionType,
			Reason:       reason,
		},
	}), nil
}

// UnassignSlot releases a slot assignment and moves the lot back to QC_APPROVED.
// This allows warehouse staff or supervisors to undo auto-assignments or reassign.
func (s *WarehouseService) UnassignSlot(ctx context.Context, req *connect.Request[whv1.UnassignSlotRequest]) (*connect.Response[whv1.UnassignSlotResponse], error) {
	msg := req.Msg
	if msg.LotId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id is required"))
	}
	if msg.Reason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("reason is required for unassign"))
	}

	actor := userFromCtx(ctx)

	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// Get lot for update
	lot, err := qtx.GetLotForUpdate(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}

	// Verify lot is in WAREHOUSE_ASSIGNED or READY_FOR_PRODUCTION
	if lot.Status != db.LotsStatusWAREHOUSEASSIGNED && lot.Status != db.LotsStatusREADYFORPRODUCTION {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("lot must be WAREHOUSE_ASSIGNED or READY_FOR_PRODUCTION (current: %s)", lot.Status))
	}

	// Get active assignment
	assignment, err := qtx.GetActiveWarehouseAssignment(ctx, msg.LotId)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no active assignment for lot"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Get location
	loc, err := qtx.GetWarehouseLocation(ctx, assignment.LocationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Increment capacity back
	if _, err := qtx.IncrementLocationCapacity(ctx, assignment.LocationID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("increment capacity: %w", err))
	}

	// Update location status back to AVAILABLE if it was OCCUPIED
	if loc.CurrentStatus == db.WarehouseLocationsCurrentStatusOCCUPIED && loc.Capacity > 0 {
		if err := qtx.UpdateLocationStatus(ctx, db.UpdateLocationStatusParams{
			CurrentStatus: db.WarehouseLocationsCurrentStatusAVAILABLE,
			ID:            assignment.LocationID,
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update location status: %w", err))
		}
	}

	// Release the assignment
	if err := qtx.ReleaseWarehouseAssignment(ctx, db.ReleaseWarehouseAssignmentParams{
		ID:     assignment.ID,
		Status: db.WarehouseAssignmentsStatusRELEASED,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("release assignment: %w", err))
	}

	// Log the unassign as OVERRIDE decision type
	decisionID := uuid.NewString()
	if err := qtx.CreateSlotDecision(ctx, db.CreateSlotDecisionParams{
		ID:           decisionID,
		LotID:        msg.LotId,
		LocationID:   assignment.LocationID,
		DecisionType: "OVERRIDE",
		Reason:       sql.NullString{String: msg.Reason, Valid: true},
		ActorID:      actor,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create slot decision: %w", err))
	}

	// Move lot back to QC_APPROVED
	if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatusQCAPPROVED,
		ID:     msg.LotId,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	// Emit events
	envelope, err := events.NewEnvelope(
		"warehouse.slot_unassigned",
		actor,
		lot.CreatedBy,
		msg.LotId,
		map[string]any{
			"lot_id":      msg.LotId,
			"lot_number": lot.LotNumber,
			"reason":      msg.Reason,
			"actor_id":    actor,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "warehouse.slot_unassigned",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	return connect.NewResponse(&whv1.UnassignSlotResponse{
		Assignment: &whv1.WarehouseAssignment{
			Id:           assignment.ID,
			LotId:        msg.LotId,
			LocationId:   assignment.LocationID,
			LocationCode: loc.Code,
			AssignedBy:   assignment.AssignedBy,
			AssignedAt:   timestamppb.New(assignment.AssignedAt),
			Status:       whv1.AssignmentStatus_ASSIGNMENT_STATUS_RELEASED,
			DecisionType: whv1.DecisionType_DECISION_TYPE_OVERRIDE,
			Reason:       msg.Reason,
		},
		LotStatus: "QC_APPROVED",
	}), nil
}

// ListSlotDecisions returns the audit trail of slot decisions for a lot.
func (s *WarehouseService) ListSlotDecisions(ctx context.Context, req *connect.Request[whv1.ListSlotDecisionsRequest]) (*connect.Response[whv1.ListSlotDecisionsResponse], error) {
	if req.Msg.LotId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id is required"))
	}

	decisions, err := s.q.ListSlotDecisionsByLot(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protos := make([]*whv1.SlotDecision, len(decisions))
	for i, d := range decisions {
		loc, err := s.q.GetWarehouseLocation(ctx, d.LocationID)
		locCode := ""
		if err == nil {
			locCode = loc.Code
		}
		protos[i] = &whv1.SlotDecision{
			Id:            d.ID,
			LotId:         d.LotID,
			LocationId:    d.LocationID,
			LocationCode:  locCode,
			DecisionType:  decisionTypeFromDB(d.DecisionType),
			Reason:        d.Reason.String,
			ActorId:       d.ActorID,
			CreatedAt:     timestamppb.New(d.CreatedAt),
		}
	}

	return connect.NewResponse(&whv1.ListSlotDecisionsResponse{
		Decisions: protos,
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
			Id:           a.ID,
			LotId:        a.LotID,
			LocationId:   a.LocationID,
			AssignedBy:   a.AssignedBy,
			AssignedAt:   timestamppb.New(a.AssignedAt),
			DecisionType: decisionTypeFromDB(a.DecisionType),
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

// jsonArrayIsEmpty reports whether a JSON array column holds no elements
// (or is null/unparseable). Used to treat an empty hazard_allowed list as
// "no segregation restriction" rather than "nothing allowed".
func jsonArrayIsEmpty(raw json.RawMessage) bool {
	var arr []string
	if json.Unmarshal(raw, &arr) != nil {
		return true
	}
	return len(arr) == 0
}

func decisionTypeToDB(dt whv1.DecisionType) string {
	switch dt {
	case whv1.DecisionType_DECISION_TYPE_AUTO:
		return "AUTO"
	case whv1.DecisionType_DECISION_TYPE_MANUAL:
		return "MANUAL"
	case whv1.DecisionType_DECISION_TYPE_OVERRIDE:
		return "OVERRIDE"
	default:
		return "MANUAL"
	}
}

func decisionTypeFromDB(s string) whv1.DecisionType {
	switch s {
	case "AUTO":
		return whv1.DecisionType_DECISION_TYPE_AUTO
	case "MANUAL":
		return whv1.DecisionType_DECISION_TYPE_MANUAL
	case "OVERRIDE":
		return whv1.DecisionType_DECISION_TYPE_OVERRIDE
	default:
		return whv1.DecisionType_DECISION_TYPE_UNSPECIFIED
	}
}
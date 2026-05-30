package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	lotv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1/lotv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
)

var _ lotv1connect.LotServiceHandler = (*LotService)(nil)

type LotService struct {
	q   *db.Queries
	dbx *sql.DB
}

func NewLotService(q *db.Queries, dbx *sql.DB) *LotService {
	return &LotService{q: q, dbx: dbx}
}

func (s *LotService) CreateLot(ctx context.Context, req *connect.Request[lotv1.CreateLotRequest]) (*connect.Response[lotv1.CreateLotResponse], error) {
	msg := req.Msg

	// Validate inputs
	if strings.TrimSpace(msg.SupplierName) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("supplier_name is required"))
	}
	if strings.TrimSpace(msg.MaterialName) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("material_name is required"))
	}
	if msg.Quantity <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("quantity must be > 0"))
	}
	if msg.MaterialType == lotv1.MaterialType_MATERIAL_TYPE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("material_type is required"))
	}
	if msg.StorageRequirement == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("storage_requirement is required"))
	}
	if strings.TrimSpace(msg.Unit) == "" {
		msg.Unit = "kg"
	}
	if strings.TrimSpace(msg.ArrivalDate) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("arrival_date is required"))
	}

	// Generate lot number with collision retry: LOT-YYYY-MM-DD-XXXXXX (6 random chars)
	now := time.Now()
	// Lot number format: LOT-YYYY-MM-DD-XXXXXXXX (8 hex chars).
	// Birthday-paradox: at 1000 lots/day, collision probability is ~0.012%/day
	// (~1/27 years). The DB UNIQUE constraint catches the residual case.
	lotNumber := fmt.Sprintf("LOT-%s-%s",
		now.Format("2006-01-02"),
		strings.ToUpper(uuid.NewString()[:8]),
	)

	// Marshal storage requirement
	sr, _ := json.Marshal(map[string]interface{}{
		"temperature_range": msg.StorageRequirement.GetTemperatureRange().String(),
		"hazard_class":      msg.StorageRequirement.GetHazardClass().String(),
	})

	id := uuid.NewString()
	arrivalDate, _ := time.Parse("2006-01-02", msg.ArrivalDate)
	createdBy := userFromCtx(ctx)

	// Wrap lot insert + outbox event in a transaction so a downstream consumer
	// (SSE / AI worker) never sees a lot that doesn't exist, and we never lose
	// a lot.created event after writing the row.
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	if err := qtx.CreateLot(ctx, db.CreateLotParams{
		ID:                 id,
		LotNumber:          lotNumber,
		SupplierName:       msg.SupplierName,
		MaterialName:       msg.MaterialName,
		MaterialType:       db.LotsMaterialType(materialTypeToDB(msg.MaterialType)),
		Quantity:           fmt.Sprintf("%.3f", msg.Quantity),
		Unit:               msg.Unit,
		ArrivalDate:        arrivalDate,
		StorageRequirement: sr,
		CreatedBy:          createdBy,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	envelope, err := events.NewEnvelope(
		"lot.created",
		createdBy,
		createdBy, // owner == creator for new lots
		id,
		map[string]any{
			"lot_id":         id,
			"lot_number":     lotNumber,
			"supplier_name":  msg.SupplierName,
			"material_name":  msg.MaterialName,
			"material_type":  string(materialTypeToDB(msg.MaterialType)),
			"quantity":       msg.Quantity,
			"unit":           msg.Unit,
			"arrival_date":   msg.ArrivalDate,
			"created_by":     createdBy,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "lot.created",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	lot, err := s.q.GetLot(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	middleware.IncLotCreated()
	return connect.NewResponse(&lotv1.CreateLotResponse{Lot: dbLotToProto(lot)}), nil
}

func (s *LotService) GetLot(ctx context.Context, req *connect.Request[lotv1.GetLotRequest]) (*connect.Response[lotv1.GetLotResponse], error) {
	lot, err := s.q.GetLot(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	return connect.NewResponse(&lotv1.GetLotResponse{Lot: dbLotToProto(lot)}), nil
}

func (s *LotService) ListLots(ctx context.Context, req *connect.Request[lotv1.ListLotsRequest]) (*connect.Response[lotv1.ListLotsResponse], error) {
	pageSize := int32(20)
	if req.Msg.PageSize > 0 {
		pageSize = req.Msg.PageSize
	}
	offset := int32(0)
	// Simple page token = offset as string
	if req.Msg.PageToken != "" {
		fmt.Sscanf(req.Msg.PageToken, "%d", &offset)
	}

	var lots []db.Lot
	var err error

	if req.Msg.StatusFilter != lotv1.LotStatus_LOT_STATUS_UNSPECIFIED {
		lots, err = s.q.ListLotsByStatus(ctx, db.ListLotsByStatusParams{
			Status: db.LotsStatus(lotStatusToDB(req.Msg.StatusFilter)),
			Limit:  pageSize,
			Offset: offset,
		})
	} else {
		lots, err = s.q.ListLots(ctx, db.ListLotsParams{
			Limit:  pageSize,
			Offset: offset,
		})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protoLots := make([]*lotv1.Lot, len(lots))
	for i, l := range lots {
		protoLots[i] = dbLotToProto(l)
	}

	nextToken := ""
	if int32(len(lots)) == pageSize {
		nextToken = fmt.Sprintf("%d", offset+pageSize)
	}

	return connect.NewResponse(&lotv1.ListLotsResponse{
		Lots:          protoLots,
		NextPageToken: nextToken,
	}), nil
}

// allowedTransitions maps each lot status to the set of statuses it can transition to.
// BLOCKED is reachable from any state. Other transitions follow the workflow FSM.
var allowedTransitions = map[lotv1.LotStatus]map[lotv1.LotStatus]bool{
	lotv1.LotStatus_LOT_STATUS_DRAFT: {
		lotv1.LotStatus_LOT_STATUS_PENDING_QC: true,
		lotv1.LotStatus_LOT_STATUS_BLOCKED:    true,
	},
	lotv1.LotStatus_LOT_STATUS_PENDING_QC: {
		lotv1.LotStatus_LOT_STATUS_AI_PROCESSING: true,
		lotv1.LotStatus_LOT_STATUS_QC_REVIEW:     true,
		lotv1.LotStatus_LOT_STATUS_DRAFT:         true,
		lotv1.LotStatus_LOT_STATUS_BLOCKED:       true,
	},
	lotv1.LotStatus_LOT_STATUS_AI_PROCESSING: {
		lotv1.LotStatus_LOT_STATUS_QC_REVIEW: true,
		lotv1.LotStatus_LOT_STATUS_BLOCKED:   true,
	},
	lotv1.LotStatus_LOT_STATUS_QC_REVIEW: {
		lotv1.LotStatus_LOT_STATUS_QC_APPROVED: true,
		lotv1.LotStatus_LOT_STATUS_QC_REJECTED: true,
		lotv1.LotStatus_LOT_STATUS_PENDING_QC:  true, // recheck
		lotv1.LotStatus_LOT_STATUS_BLOCKED:     true,
	},
	lotv1.LotStatus_LOT_STATUS_QC_APPROVED: {
		lotv1.LotStatus_LOT_STATUS_WAREHOUSE_ASSIGNED:   true,
		lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION: true, // direct assignment
		lotv1.LotStatus_LOT_STATUS_BLOCKED:              true,
	},
	lotv1.LotStatus_LOT_STATUS_QC_REJECTED: {
		lotv1.LotStatus_LOT_STATUS_PENDING_QC: true, // re-upload + retry
		lotv1.LotStatus_LOT_STATUS_BLOCKED:    true,
	},
	lotv1.LotStatus_LOT_STATUS_WAREHOUSE_ASSIGNED: {
		lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION: true,
		lotv1.LotStatus_LOT_STATUS_BLOCKED:              true,
	},
	lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION: {
		lotv1.LotStatus_LOT_STATUS_BLOCKED: true,
	},
	lotv1.LotStatus_LOT_STATUS_BLOCKED: {}, // terminal
}

func (s *LotService) UpdateLotStatus(ctx context.Context, req *connect.Request[lotv1.UpdateLotStatusRequest]) (*connect.Response[lotv1.UpdateLotStatusResponse], error) {
	if req.Msg.LotId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id is required"))
	}
	if req.Msg.NewStatus == lotv1.LotStatus_LOT_STATUS_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_status is required"))
	}

	actor := userFromCtx(ctx)

	// Open the transaction first, then read the lot WITH FOR UPDATE so the
	// row stays locked through validation and the write below. Without this,
	// two concurrent UpdateLotStatus calls could both pass the FSM check
	// against the same source state, then both write — corrupting the FSM.
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	current, err := qtx.GetLotForUpdate(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	currentStatus := lotStatusFromDB(string(current.Status))

	// Same-state is idempotent: succeed without mutation. Skip both the DB write
	// and the outbox event so we don't spam consumers with no-op updates.
	if currentStatus == req.Msg.NewStatus {
		// Commit (no-op) so the FOR UPDATE lock is released cleanly.
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit no-op: %w", err))
		}
		return connect.NewResponse(&lotv1.UpdateLotStatusResponse{Lot: dbLotToProto(current)}), nil
	}

	allowed, ok := allowedTransitions[currentStatus]
	if !ok || !allowed[req.Msg.NewStatus] {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("invalid lot status transition: %s → %s",
				currentStatus, req.Msg.NewStatus))
	}

	if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatus(lotStatusToDB(req.Msg.NewStatus)),
		ID:     req.Msg.LotId,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	envelope, err := events.NewEnvelope(
		"lot.status_changed",
		actor,
		current.CreatedBy, // owner is always the lot creator regardless of who changes status
		req.Msg.LotId,
		map[string]any{
			"lot_id":     req.Msg.LotId,
			"lot_number": current.LotNumber,
			"from":       lotStatusToDB(currentStatus),
			"to":         lotStatusToDB(req.Msg.NewStatus),
			"reason":     req.Msg.Reason,
			"created_by": current.CreatedBy,
			"actor_id":   actor,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "lot.status_changed",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	// A direct transition to READY_FOR_PRODUCTION (not via warehouse assignment)
	// still emits the dedicated production-handoff event so dispatch/PPIC
	// consumers get a uniform signal regardless of which path reached it.
	if req.Msg.NewStatus == lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION {
		if err := emitLotReadyForProduction(ctx, qtx, actor, current, ""); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	lot, err := s.q.GetLot(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	return connect.NewResponse(&lotv1.UpdateLotStatusResponse{Lot: dbLotToProto(lot)}), nil
}

func (s *LotService) GetLotTimeline(ctx context.Context, req *connect.Request[lotv1.GetLotTimelineRequest]) (*connect.Response[lotv1.GetLotTimelineResponse], error) {
	if req.Msg.LotId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id is required"))
	}
	logs, err := s.q.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: "lot",
		EntityID:   req.Msg.LotId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list audit logs: %w", err))
	}
	entries := make([]*lotv1.TimelineEntry, 0, len(logs))
	for _, l := range logs {
		entries = append(entries, &lotv1.TimelineEntry{
			Id:          l.ID,
			Action:      l.Action,
			ActorUserId: l.ActorUserID,
			ActorRole:   l.ActorRole,
			CreatedAt:   timestamppb.New(l.CreatedAt),
		})
	}
	return connect.NewResponse(&lotv1.GetLotTimelineResponse{Entries: entries}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────

// emitLotReadyForProduction writes the dedicated `lot.ready_for_production`
// production-handoff outbox event inside the caller's transaction. It carries
// the full lot payload (material, quantity, storage requirement, slot) that a
// downstream dispatch / PPIC consumer needs — a distinct subject that can be
// granted via the SSE role filter without exposing the whole lot.* firehose.
// locationCode may be empty when the handoff did not originate from a slot
// assignment (e.g. a direct status update).
func emitLotReadyForProduction(ctx context.Context, qtx *db.Queries, actor string, lot db.Lot, locationCode string) error {
	var sr map[string]string
	_ = json.Unmarshal(lot.StorageRequirement, &sr)
	envelope, err := events.NewEnvelope(
		"lot.ready_for_production",
		actor,
		lot.CreatedBy,
		lot.ID,
		map[string]any{
			"lot_id":            lot.ID,
			"lot_number":        lot.LotNumber,
			"supplier_name":     lot.SupplierName,
			"material_name":     lot.MaterialName,
			"material_type":     string(lot.MaterialType),
			"quantity":          parseFloat(lot.Quantity),
			"unit":              lot.Unit,
			"temperature_range": sr["temperature_range"],
			"hazard_class":      sr["hazard_class"],
			"location_code":     locationCode,
			"created_by":        lot.CreatedBy,
			"actor_id":          actor,
		},
	)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("build ready_for_production envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "lot.ready_for_production",
		PayloadJson: envelope,
	}); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("create ready_for_production outbox event: %w", err))
	}
	return nil
}

func dbLotToProto(l db.Lot) *lotv1.Lot {
	var sr lotv1.StorageRequirement
	var raw map[string]string
	if json.Unmarshal(l.StorageRequirement, &raw) == nil {
		sr.TemperatureRange = tempRangeFromString(raw["temperature_range"])
		sr.HazardClass = hazardClassFromString(raw["hazard_class"])
	}

	return &lotv1.Lot{
		Id:                 l.ID,
		LotNumber:          l.LotNumber,
		SupplierName:       l.SupplierName,
		MaterialName:       l.MaterialName,
		MaterialType:       materialTypeFromDB(string(l.MaterialType)),
		Quantity:           parseFloat(l.Quantity),
		Unit:               l.Unit,
		ArrivalDate:        l.ArrivalDate.Format("2006-01-02"),
		StorageRequirement: &sr,
		Status:             lotStatusFromDB(string(l.Status)),
		CreatedBy:          l.CreatedBy,
		CreatedAt:          timestamppb.New(l.CreatedAt),
		UpdatedAt:          timestamppb.New(l.UpdatedAt),
	}
}

func materialTypeToDB(mt lotv1.MaterialType) string {
	switch mt {
	case lotv1.MaterialType_MATERIAL_TYPE_RAW_BOTANICAL:
		return "RAW_BOTANICAL"
	case lotv1.MaterialType_MATERIAL_TYPE_EXTRACT:
		return "EXTRACT"
	case lotv1.MaterialType_MATERIAL_TYPE_POWDER:
		return "POWDER"
	default:
		return "OTHER"
	}
}

func materialTypeFromDB(s string) lotv1.MaterialType {
	switch s {
	case "RAW_BOTANICAL":
		return lotv1.MaterialType_MATERIAL_TYPE_RAW_BOTANICAL
	case "EXTRACT":
		return lotv1.MaterialType_MATERIAL_TYPE_EXTRACT
	case "POWDER":
		return lotv1.MaterialType_MATERIAL_TYPE_POWDER
	default:
		return lotv1.MaterialType_MATERIAL_TYPE_OTHER
	}
}

func lotStatusToDB(s lotv1.LotStatus) string {
	switch s {
	case lotv1.LotStatus_LOT_STATUS_DRAFT:
		return "DRAFT"
	case lotv1.LotStatus_LOT_STATUS_PENDING_QC:
		return "PENDING_QC"
	case lotv1.LotStatus_LOT_STATUS_AI_PROCESSING:
		return "AI_PROCESSING"
	case lotv1.LotStatus_LOT_STATUS_QC_REVIEW:
		return "QC_REVIEW"
	case lotv1.LotStatus_LOT_STATUS_QC_APPROVED:
		return "QC_APPROVED"
	case lotv1.LotStatus_LOT_STATUS_QC_REJECTED:
		return "QC_REJECTED"
	case lotv1.LotStatus_LOT_STATUS_WAREHOUSE_ASSIGNED:
		return "WAREHOUSE_ASSIGNED"
	case lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION:
		return "READY_FOR_PRODUCTION"
	case lotv1.LotStatus_LOT_STATUS_BLOCKED:
		return "BLOCKED"
	default:
		return "DRAFT"
	}
}

func lotStatusFromDB(s string) lotv1.LotStatus {
	switch s {
	case "DRAFT":
		return lotv1.LotStatus_LOT_STATUS_DRAFT
	case "PENDING_QC":
		return lotv1.LotStatus_LOT_STATUS_PENDING_QC
	case "AI_PROCESSING":
		return lotv1.LotStatus_LOT_STATUS_AI_PROCESSING
	case "QC_REVIEW":
		return lotv1.LotStatus_LOT_STATUS_QC_REVIEW
	case "QC_APPROVED":
		return lotv1.LotStatus_LOT_STATUS_QC_APPROVED
	case "QC_REJECTED":
		return lotv1.LotStatus_LOT_STATUS_QC_REJECTED
	case "WAREHOUSE_ASSIGNED":
		return lotv1.LotStatus_LOT_STATUS_WAREHOUSE_ASSIGNED
	case "READY_FOR_PRODUCTION":
		return lotv1.LotStatus_LOT_STATUS_READY_FOR_PRODUCTION
	case "BLOCKED":
		return lotv1.LotStatus_LOT_STATUS_BLOCKED
	default:
		return lotv1.LotStatus_LOT_STATUS_UNSPECIFIED
	}
}

func tempRangeFromString(s string) lotv1.TemperatureRange {
	switch s {
	case "TEMPERATURE_RANGE_AMBIENT":
		return lotv1.TemperatureRange_TEMPERATURE_RANGE_AMBIENT
	case "TEMPERATURE_RANGE_COLD":
		return lotv1.TemperatureRange_TEMPERATURE_RANGE_COLD
	case "TEMPERATURE_RANGE_DEEP_FREEZE":
		return lotv1.TemperatureRange_TEMPERATURE_RANGE_DEEP_FREEZE
	default:
		return lotv1.TemperatureRange_TEMPERATURE_RANGE_UNSPECIFIED
	}
}

func hazardClassFromString(s string) lotv1.HazardClass {
	switch s {
	case "HAZARD_CLASS_NONE":
		return lotv1.HazardClass_HAZARD_CLASS_NONE
	case "HAZARD_CLASS_IBC":
		return lotv1.HazardClass_HAZARD_CLASS_IBC
	case "HAZARD_CLASS_IPPC":
		return lotv1.HazardClass_HAZARD_CLASS_IPPC
	default:
		return lotv1.HazardClass_HAZARD_CLASS_UNSPECIFIED
	}
}

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

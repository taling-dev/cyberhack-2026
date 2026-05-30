package handler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	dispatchv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dispatch/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dispatch/v1/dispatchv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

var _ dispatchv1connect.DispatchServiceHandler = (*DispatchService)(nil)

type DispatchService struct {
	q   *db.Queries
	dbx *sql.DB
}

func NewDispatchService(q *db.Queries, dbx *sql.DB) *DispatchService {
	return &DispatchService{q: q, dbx: dbx}
}

// dispatchTransitions is the dispatch lifecycle FSM. CANCELLED is a side-rail
// reachable from any non-terminal state; DELIVERED and CANCELLED are terminal.
var dispatchTransitions = map[dispatchv1.DispatchStatus]map[dispatchv1.DispatchStatus]bool{
	dispatchv1.DispatchStatus_DISPATCH_STATUS_PENDING: {
		dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED: true,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED: true,
	},
	dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED: {
		dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT: true,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED:  true,
	},
	dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT: {
		dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED: true,
		dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED: true,
	},
	dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED: {},
	dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED: {},
}

// CreateDispatch records a shipment for a production-ready lot. The lot must be
// in READY_FOR_PRODUCTION and may have at most one active (non-cancelled)
// dispatch. The insert + dispatch.created outbox event are written atomically.
func (s *DispatchService) CreateDispatch(ctx context.Context, req *connect.Request[dispatchv1.CreateDispatchRequest]) (*connect.Response[dispatchv1.CreateDispatchResponse], error) {
	msg := req.Msg
	if strings.TrimSpace(msg.LotId) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id is required"))
	}
	if strings.TrimSpace(msg.Destination) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("destination is required"))
	}
	if msg.Quantity <= 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("quantity must be > 0"))
	}
	unit := strings.TrimSpace(msg.Unit)
	if unit == "" {
		unit = "kg"
	}

	var scheduledAt sql.NullTime
	if strings.TrimSpace(msg.ScheduledAt) != "" {
		ts, err := time.Parse(time.RFC3339, msg.ScheduledAt)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("scheduled_at must be RFC3339"))
		}
		scheduledAt = sql.NullTime{Time: ts, Valid: true}
	}

	createdBy := userFromCtx(ctx)
	id := uuid.NewString()
	now := time.Now()
	dispatchNumber := fmt.Sprintf("DSP-%s-%s", now.Format("2006-01-02"), strings.ToUpper(uuid.NewString()[:8]))

	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// The lot must exist and be production-ready. Lock it so a concurrent lot
	// transition can't move it out of READY_FOR_PRODUCTION between our check
	// and the dispatch insert.
	lot, err := qtx.GetLotForUpdate(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	if lot.Status != db.LotsStatusREADYFORPRODUCTION {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("lot must be READY_FOR_PRODUCTION (current: %s)", lot.Status))
	}

	// One active dispatch per lot — reject a second shipment of the same lot.
	active, err := qtx.CountActiveDispatchesForLot(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("count dispatches: %w", err))
	}
	if active > 0 {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("lot already has an active dispatch"))
	}

	if err := qtx.CreateDispatch(ctx, db.CreateDispatchParams{
		ID:             id,
		DispatchNumber: dispatchNumber,
		LotID:          msg.LotId,
		Destination:    msg.Destination,
		Carrier:        strings.TrimSpace(msg.Carrier),
		Quantity:       fmt.Sprintf("%.3f", msg.Quantity),
		Unit:           unit,
		ScheduledAt:    scheduledAt,
		Notes:          sql.NullString{String: msg.Notes, Valid: msg.Notes != ""},
		CreatedBy:      createdBy,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create dispatch: %w", err))
	}

	envelope, err := events.NewEnvelope(
		"dispatch.created",
		createdBy,
		lot.CreatedBy, // owner is the lot creator so operators see their lot's dispatch
		id,
		map[string]any{
			"dispatch_id":     id,
			"dispatch_number": dispatchNumber,
			"lot_id":          msg.LotId,
			"lot_number":      lot.LotNumber,
			"destination":     msg.Destination,
			"carrier":         msg.Carrier,
			"status":          "PENDING",
			"created_by":      createdBy,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "dispatch.created",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	created, err := s.q.GetDispatch(ctx, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&dispatchv1.CreateDispatchResponse{Dispatch: dbDispatchToProto(created, lot.LotNumber)}), nil
}

func (s *DispatchService) GetDispatch(ctx context.Context, req *connect.Request[dispatchv1.GetDispatchRequest]) (*connect.Response[dispatchv1.GetDispatchResponse], error) {
	d, err := s.q.GetDispatch(ctx, req.Msg.DispatchId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("dispatch not found"))
	}
	return connect.NewResponse(&dispatchv1.GetDispatchResponse{Dispatch: dbDispatchToProto(d, s.lotNumber(ctx, d.LotID))}), nil
}

func (s *DispatchService) ListDispatches(ctx context.Context, req *connect.Request[dispatchv1.ListDispatchesRequest]) (*connect.Response[dispatchv1.ListDispatchesResponse], error) {
	pageSize := int32(20)
	if req.Msg.PageSize > 0 {
		pageSize = req.Msg.PageSize
	}
	var offset int32
	if req.Msg.PageToken != "" {
		fmt.Sscanf(req.Msg.PageToken, "%d", &offset)
	}

	var rows []db.Dispatch
	var err error
	switch {
	case req.Msg.LotIdFilter != "":
		rows, err = s.q.ListDispatchesByLot(ctx, db.ListDispatchesByLotParams{LotID: req.Msg.LotIdFilter, Limit: pageSize, Offset: offset})
	case req.Msg.StatusFilter != dispatchv1.DispatchStatus_DISPATCH_STATUS_UNSPECIFIED:
		rows, err = s.q.ListDispatchesByStatus(ctx, db.ListDispatchesByStatusParams{Status: dispatchStatusToDB(req.Msg.StatusFilter), Limit: pageSize, Offset: offset})
	default:
		rows, err = s.q.ListDispatches(ctx, db.ListDispatchesParams{Limit: pageSize, Offset: offset})
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	total, _ := s.q.CountDispatches(ctx)
	protos := make([]*dispatchv1.Dispatch, len(rows))
	for i, d := range rows {
		protos[i] = dbDispatchToProto(d, s.lotNumber(ctx, d.LotID))
	}
	nextToken := ""
	if int32(len(rows)) == pageSize {
		nextToken = fmt.Sprintf("%d", offset+pageSize)
	}
	return connect.NewResponse(&dispatchv1.ListDispatchesResponse{
		Dispatches:    protos,
		NextPageToken: nextToken,
		TotalCount:    int32(total),
	}), nil
}

// UpdateDispatchStatus advances a dispatch through its FSM. The row is locked
// FOR UPDATE so concurrent transitions can't both pass the FSM check, and the
// update + dispatch.status_changed outbox event are written atomically.
func (s *DispatchService) UpdateDispatchStatus(ctx context.Context, req *connect.Request[dispatchv1.UpdateDispatchStatusRequest]) (*connect.Response[dispatchv1.UpdateDispatchStatusResponse], error) {
	if req.Msg.DispatchId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("dispatch_id is required"))
	}
	if req.Msg.NewStatus == dispatchv1.DispatchStatus_DISPATCH_STATUS_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("new_status is required"))
	}

	actor := userFromCtx(ctx)

	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	current, err := qtx.GetDispatchForUpdate(ctx, req.Msg.DispatchId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("dispatch not found"))
	}
	currentStatus := dispatchStatusFromDB(current.Status)

	// Same-state is idempotent: succeed without mutation or event.
	if currentStatus == req.Msg.NewStatus {
		if err := tx.Commit(); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit no-op: %w", err))
		}
		return connect.NewResponse(&dispatchv1.UpdateDispatchStatusResponse{Dispatch: dbDispatchToProto(current, s.lotNumber(ctx, current.LotID))}), nil
	}

	allowed, ok := dispatchTransitions[currentStatus]
	if !ok || !allowed[req.Msg.NewStatus] {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("invalid dispatch status transition: %s → %s", currentStatus, req.Msg.NewStatus))
	}

	if err := qtx.UpdateDispatchStatus(ctx, db.UpdateDispatchStatusParams{
		Status: dispatchStatusToDB(req.Msg.NewStatus),
		ID:     req.Msg.DispatchId,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Resolve the owner (lot creator) for owner-scoped SSE fan-out.
	ownerID := actor
	lotNumber := ""
	if lot, err := qtx.GetLot(ctx, current.LotID); err == nil {
		ownerID = lot.CreatedBy
		lotNumber = lot.LotNumber
	}

	envelope, err := events.NewEnvelope(
		"dispatch.status_changed",
		actor,
		ownerID,
		req.Msg.DispatchId,
		map[string]any{
			"dispatch_id":     req.Msg.DispatchId,
			"dispatch_number": current.DispatchNumber,
			"lot_id":          current.LotID,
			"lot_number":      lotNumber,
			"from":            string(dispatchStatusToDB(currentStatus)),
			"to":              string(dispatchStatusToDB(req.Msg.NewStatus)),
			"reason":          req.Msg.Reason,
			"actor_id":        actor,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "dispatch.status_changed",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

	updated, err := s.q.GetDispatch(ctx, req.Msg.DispatchId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&dispatchv1.UpdateDispatchStatusResponse{Dispatch: dbDispatchToProto(updated, lotNumber)}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────

func (s *DispatchService) lotNumber(ctx context.Context, lotID string) string {
	if lot, err := s.q.GetLot(ctx, lotID); err == nil {
		return lot.LotNumber
	}
	return ""
}

func dbDispatchToProto(d db.Dispatch, lotNumber string) *dispatchv1.Dispatch {
	p := &dispatchv1.Dispatch{
		Id:             d.ID,
		DispatchNumber: d.DispatchNumber,
		LotId:          d.LotID,
		LotNumber:      lotNumber,
		Destination:    d.Destination,
		Carrier:        d.Carrier,
		Quantity:       parseFloat(d.Quantity),
		Unit:           d.Unit,
		Notes:          d.Notes.String,
		Status:         dispatchStatusFromDB(d.Status),
		CreatedBy:      d.CreatedBy,
		CreatedAt:      timestamppb.New(d.CreatedAt),
		UpdatedAt:      timestamppb.New(d.UpdatedAt),
	}
	if d.ScheduledAt.Valid {
		p.ScheduledAt = timestamppb.New(d.ScheduledAt.Time)
	}
	return p
}

func dispatchStatusToDB(s dispatchv1.DispatchStatus) db.DispatchesStatus {
	switch s {
	case dispatchv1.DispatchStatus_DISPATCH_STATUS_PENDING:
		return db.DispatchesStatusPENDING
	case dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED:
		return db.DispatchesStatusSCHEDULED
	case dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT:
		return db.DispatchesStatusINTRANSIT
	case dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED:
		return db.DispatchesStatusDELIVERED
	case dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED:
		return db.DispatchesStatusCANCELLED
	default:
		return db.DispatchesStatusPENDING
	}
}

func dispatchStatusFromDB(s db.DispatchesStatus) dispatchv1.DispatchStatus {
	switch s {
	case db.DispatchesStatusPENDING:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_PENDING
	case db.DispatchesStatusSCHEDULED:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_SCHEDULED
	case db.DispatchesStatusINTRANSIT:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_IN_TRANSIT
	case db.DispatchesStatusDELIVERED:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_DELIVERED
	case db.DispatchesStatusCANCELLED:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_CANCELLED
	default:
		return dispatchv1.DispatchStatus_DISPATCH_STATUS_UNSPECIFIED
	}
}

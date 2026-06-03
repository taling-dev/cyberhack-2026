package handler

import (
	"context"
	"database/sql"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	rv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/review/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/review/v1/reviewv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
)

var _ reviewv1connect.ReviewRequestServiceHandler = (*ReviewRequestService)(nil)

type ReviewRequestService struct {
	q *db.Queries
}

func NewReviewRequestService(q *db.Queries) *ReviewRequestService {
	return &ReviewRequestService{q: q}
}

func (s *ReviewRequestService) CreateReviewRequest(ctx context.Context, req *connect.Request[rv1.CreateReviewRequestRequest]) (*connect.Response[rv1.CreateReviewRequestResponse], error) {
	msg := req.Msg

	if msg.LotId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("lot_id is required"))
	}
	if msg.Reason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("reason is required"))
	}
	if msg.TargetRole == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_role is required"))
	}

	// Verify lot exists
	lot, err := s.q.GetLot(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("lot not found"))
	}

	requesterID := userFromCtx(ctx)
	requesterRole := "" // Default to empty if no role found

	requestID := uuid.NewString()
	requestType := requestTypeToDB(msg.RequestType)

	// Create the review request
	if err := s.q.CreateReviewRequest(ctx, db.CreateReviewRequestParams{
		ID:           requestID,
		LotID:        msg.LotId,
		RequesterID:  requesterID,
		RequesterRole: requesterRole,
		TargetRole:   msg.TargetRole,
		RequestType: requestType,
		Reason:      msg.Reason,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Emit event for real-time notification
	envelope, err := events.NewEnvelope(
		"review_request.created",
		requesterID,
		lot.CreatedBy,
		msg.LotId,
		map[string]any{
			"request_id":     requestID,
			"lot_id":         msg.LotId,
			"lot_number":     lot.LotNumber,
			"request_type":   requestType,
			"target_role":    msg.TargetRole,
			"reason":         msg.Reason,
			"requester_id":   requesterID,
			"requester_role": requesterRole,
		},
	)
	if err == nil {
		// Create outbox event (best effort)
		_ = s.q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			ID:          uuid.NewString(),
			EventType:   "review_request.created",
			PayloadJson: envelope,
		})
	}

	return connect.NewResponse(&rv1.CreateReviewRequestResponse{
		Request: &rv1.ReviewRequest{
			Id:           requestID,
			LotId:        msg.LotId,
			LotNumber:    lot.LotNumber,
			RequestType:  msg.RequestType,
			RequesterId:  requesterID,
			RequesterRole: requesterRole,
			TargetRole:   msg.TargetRole,
			Reason:       msg.Reason,
			Status:       rv1.ReviewStatus_REVIEW_STATUS_PENDING,
		},
	}), nil
}

func (s *ReviewRequestService) ListReviewRequests(ctx context.Context, req *connect.Request[rv1.ListReviewRequestsRequest]) (*connect.Response[rv1.ListReviewRequestsResponse], error) {
	pageSize := int32(20)
	if req.Msg.PageSize > 0 {
		pageSize = req.Msg.PageSize
	}

	// Build filter based on request
	statusFilter := db.ReviewRequestsStatusPENDING
	if req.Msg.StatusFilter != rv1.ReviewStatus_REVIEW_STATUS_UNSPECIFIED {
		statusFilter = reviewStatusToDB(req.Msg.StatusFilter)
	}

	requests, err := s.q.ListReviewRequests(ctx, db.ListReviewRequestsParams{
		Limit:  pageSize,
		Offset: 0,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Filter by status if specified
	var filtered []db.ReviewRequest
	if req.Msg.StatusFilter != rv1.ReviewStatus_REVIEW_STATUS_UNSPECIFIED {
		for _, r := range requests {
			if r.Status == statusFilter {
				filtered = append(filtered, r)
			}
		}
		requests = filtered
	}

	// Get lot numbers for each request
	protos := make([]*rv1.ReviewRequest, len(requests))
	for i, r := range requests {
		lot, _ := s.q.GetLot(ctx, r.LotID)
		lotNumber := ""
		if lot.ID != "" {
			lotNumber = lot.LotNumber
		}

		protos[i] = &rv1.ReviewRequest{
			Id:            r.ID,
			LotId:         r.LotID,
			LotNumber:     lotNumber,
			RequestType:   requestTypeFromDB(r.RequestType),
			RequesterId:   r.RequesterID,
			RequesterRole: r.RequesterRole,
			TargetRole:    r.TargetRole,
			Reason:        r.Reason,
			Status:        reviewStatusFromDB(string(r.Status)),
			ReviewedBy:    r.ReviewedBy.String,
			ReviewNote:    r.ReviewNote.String,
		}
	}

	return connect.NewResponse(&rv1.ListReviewRequestsResponse{
		Requests: protos,
	}), nil
}

func (s *ReviewRequestService) ResolveReviewRequest(ctx context.Context, req *connect.Request[rv1.ResolveReviewRequestRequest]) (*connect.Response[rv1.ResolveReviewRequestResponse], error) {
	msg := req.Msg

	if msg.RequestId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("request_id is required"))
	}

	reviewer := userFromCtx(ctx)

	// Get the request
	reviewReq, err := s.q.GetReviewRequest(ctx, msg.RequestId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("review request not found"))
	}

	// Verify it's pending
	if reviewReq.Status != db.ReviewRequestsStatusPENDING {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("review request is not pending"))
	}

	// Determine new status
	newStatus := db.ReviewRequestsStatusREJECTED
	if msg.Approved {
		newStatus = db.ReviewRequestsStatusAPPROVED
	}

	// Update the request
	reviewNote := ""
	if msg.ReviewNote != "" {
		reviewNote = msg.ReviewNote
	}
	if err := s.q.UpdateReviewRequestStatus(ctx, db.UpdateReviewRequestStatusParams{
		Status:     newStatus,
		ReviewedBy: reviewer,
		ReviewNote: sql.NullString{String: reviewNote, Valid: reviewNote != ""},
		ID:         msg.RequestId,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Get lot for event
	lot, _ := s.q.GetLot(ctx, reviewReq.LotID)
	lotNumber := ""
	if lot.ID != "" {
		lotNumber = lot.LotNumber
	}

	// Emit event
	envelope, _ := events.NewEnvelope(
		"review_request.resolved",
		reviewer,
		lot.CreatedBy,
		reviewReq.LotID,
		map[string]any{
			"request_id":  msg.RequestId,
			"lot_id":      reviewReq.LotID,
			"lot_number":  lotNumber,
			"approved":    msg.Approved,
			"review_note": msg.ReviewNote,
			"reviewed_by": reviewer,
		},
	)
	_ = s.q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "review_request.resolved",
		PayloadJson: envelope,
	})

	return connect.NewResponse(&rv1.ResolveReviewRequestResponse{
		Request: &rv1.ReviewRequest{
			Id:           reviewReq.ID,
			LotId:        reviewReq.LotID,
			LotNumber:    lotNumber,
			RequestType:  requestTypeFromDB(reviewReq.RequestType),
			RequesterId:  reviewReq.RequesterID,
			RequesterRole: reviewReq.RequesterRole,
			TargetRole:   reviewReq.TargetRole,
			Reason:       reviewReq.Reason,
			Status:       reviewStatusFromDB(string(newStatus)),
			ReviewedBy:   reviewer,
			ReviewNote:   msg.ReviewNote,
		},
	}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────

func requestTypeToDB(rt rv1.RequestType) string {
	switch rt {
	case rv1.RequestType_REQUEST_TYPE_WAREHOUSE_REASSIGN:
		return "WAREHOUSE_REASSIGN"
	case rv1.RequestType_REQUEST_TYPE_STATUS_CHANGE:
		return "STATUS_CHANGE"
	default:
		return "WAREHOUSE_REASSIGN"
	}
}

func requestTypeFromDB(s string) rv1.RequestType {
	switch s {
	case "WAREHOUSE_REASSIGN":
		return rv1.RequestType_REQUEST_TYPE_WAREHOUSE_REASSIGN
	case "STATUS_CHANGE":
		return rv1.RequestType_REQUEST_TYPE_STATUS_CHANGE
	default:
		return rv1.RequestType_REQUEST_TYPE_UNSPECIFIED
	}
}

func reviewStatusToDB(rs rv1.ReviewStatus) db.ReviewRequestsStatus {
	switch rs {
	case rv1.ReviewStatus_REVIEW_STATUS_PENDING:
		return db.ReviewRequestsStatusPENDING
	case rv1.ReviewStatus_REVIEW_STATUS_APPROVED:
		return db.ReviewRequestsStatusAPPROVED
	case rv1.ReviewStatus_REVIEW_STATUS_REJECTED:
		return db.ReviewRequestsStatusREJECTED
	case rv1.ReviewStatus_REVIEW_STATUS_CANCELLED:
		return db.ReviewRequestsStatusCANCELLED
	default:
		return db.ReviewRequestsStatusPENDING
	}
}

func reviewStatusFromDB(s string) rv1.ReviewStatus {
	switch s {
	case "PENDING":
		return rv1.ReviewStatus_REVIEW_STATUS_PENDING
	case "APPROVED":
		return rv1.ReviewStatus_REVIEW_STATUS_APPROVED
	case "REJECTED":
		return rv1.ReviewStatus_REVIEW_STATUS_REJECTED
	case "CANCELLED":
		return rv1.ReviewStatus_REVIEW_STATUS_CANCELLED
	default:
		return rv1.ReviewStatus_REVIEW_STATUS_UNSPECIFIED
	}
}
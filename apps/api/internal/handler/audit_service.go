package handler

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	auditv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/audit/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/audit/v1/auditv1connect"
)

var _ auditv1connect.AuditServiceHandler = (*AuditService)(nil)

type AuditService struct {
	q *db.Queries
}

func NewAuditService(q *db.Queries) *AuditService {
	return &AuditService{q: q}
}

func (s *AuditService) ListAuditLogs(ctx context.Context, req *connect.Request[auditv1.ListAuditLogsRequest]) (*connect.Response[auditv1.ListAuditLogsResponse], error) {
	pageSize := int32(50)
	if req.Msg.PageSize > 0 {
		pageSize = req.Msg.PageSize
	}
	offset := int32(0)
	if req.Msg.PageToken != "" {
		fmt.Sscanf(req.Msg.PageToken, "%d", &offset)
	}

	logs, err := s.q.ListAuditLogs(ctx, db.ListAuditLogsParams{Limit: pageSize, Offset: offset})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protos := make([]*auditv1.AuditLog, len(logs))
	for i, l := range logs {
		protos[i] = dbAuditToProto(l)
	}

	nextToken := ""
	if int32(len(logs)) == pageSize {
		nextToken = fmt.Sprintf("%d", offset+pageSize)
	}

	return connect.NewResponse(&auditv1.ListAuditLogsResponse{
		Logs:          protos,
		NextPageToken: nextToken,
	}), nil
}

func (s *AuditService) GetEntityAuditTrail(ctx context.Context, req *connect.Request[auditv1.GetEntityAuditTrailRequest]) (*connect.Response[auditv1.GetEntityAuditTrailResponse], error) {
	if req.Msg.EntityType == "" || req.Msg.EntityId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("entity_type and entity_id required"))
	}

	logs, err := s.q.ListAuditLogsByEntity(ctx, db.ListAuditLogsByEntityParams{
		EntityType: req.Msg.EntityType,
		EntityID:   req.Msg.EntityId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	protos := make([]*auditv1.AuditLog, len(logs))
	for i, l := range logs {
		protos[i] = dbAuditToProto(l)
	}

	return connect.NewResponse(&auditv1.GetEntityAuditTrailResponse{Entries: protos}), nil
}

func dbAuditToProto(l db.AuditLog) *auditv1.AuditLog {
	a := &auditv1.AuditLog{
		Id:          l.ID,
		ActorUserId: l.ActorUserID,
		ActorRole:   l.ActorRole,
		Action:      l.Action,
		EntityType:  l.EntityType,
		EntityId:    l.EntityID,
		CreatedAt:   timestamppb.New(l.CreatedAt),
	}
	if l.BeforeJson != nil {
		a.BeforeJson = string(l.BeforeJson)
	}
	if l.AfterJson != nil {
		a.AfterJson = string(l.AfterJson)
	}
	if l.RequestID.Valid {
		a.RequestId = l.RequestID.String
	}
	if l.TraceID.Valid {
		a.TraceId = l.TraceID.String
	}
	return a
}

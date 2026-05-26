package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	lotv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1/lotv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
)

var _ lotv1connect.LotServiceHandler = (*LotService)(nil)

type LotService struct {
	q *db.Queries
}

func NewLotService(q *db.Queries) *LotService {
	return &LotService{q: q}
}

func (s *LotService) CreateLot(ctx context.Context, req *connect.Request[lotv1.CreateLotRequest]) (*connect.Response[lotv1.CreateLotResponse], error) {
	msg := req.Msg

	// Generate lot number: LOT-YYYY-MMDD-XXX
	now := time.Now()
	lotNumber := fmt.Sprintf("LOT-%s-%03d", now.Format("2006-0102"), now.UnixMilli()%1000)

	// Marshal storage requirement
	sr, _ := json.Marshal(map[string]interface{}{
		"temperature_range": msg.StorageRequirement.GetTemperatureRange().String(),
		"hazard_class":      msg.StorageRequirement.GetHazardClass().String(),
	})

	id := uuid.NewString()
	arrivalDate, _ := time.Parse("2006-01-02", msg.ArrivalDate)

	err := s.q.CreateLot(ctx, db.CreateLotParams{
		ID:                 id,
		LotNumber:          lotNumber,
		SupplierName:       msg.SupplierName,
		MaterialName:       msg.MaterialName,
		MaterialType:       db.LotsMaterialType(materialTypeToDB(msg.MaterialType)),
		Quantity:           fmt.Sprintf("%.3f", msg.Quantity),
		Unit:               msg.Unit,
		ArrivalDate:        arrivalDate,
		StorageRequirement: sr,
		CreatedBy:          userFromCtx(ctx),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
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

func (s *LotService) UpdateLotStatus(ctx context.Context, req *connect.Request[lotv1.UpdateLotStatusRequest]) (*connect.Response[lotv1.UpdateLotStatusResponse], error) {
	err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatus(lotStatusToDB(req.Msg.NewStatus)),
		ID:     req.Msg.LotId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	lot, err := s.q.GetLot(ctx, req.Msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}
	return connect.NewResponse(&lotv1.UpdateLotStatusResponse{Lot: dbLotToProto(lot)}), nil
}

func (s *LotService) GetLotTimeline(ctx context.Context, req *connect.Request[lotv1.GetLotTimelineRequest]) (*connect.Response[lotv1.GetLotTimelineResponse], error) {
	// Implemented in Task 13 (audit middleware)
	return connect.NewResponse(&lotv1.GetLotTimelineResponse{}), nil
}

// ─── Helpers ─────────────────────────────────────────────────────

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

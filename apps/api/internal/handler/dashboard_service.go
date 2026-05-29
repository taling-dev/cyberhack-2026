package handler

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	dashv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1/dashboardv1connect"
)

var _ dashboardv1connect.DashboardServiceHandler = (*DashboardService)(nil)

type DashboardService struct {
	q *db.Queries
}

func NewDashboardService(q *db.Queries) *DashboardService {
	return &DashboardService{q: q}
}

func (s *DashboardService) GetOpsDashboard(ctx context.Context, req *connect.Request[dashv1.GetOpsDashboardRequest]) (*connect.Response[dashv1.GetOpsDashboardResponse], error) {
	// The primary query (status group counts) drives the dashboard's main
	// signal — if it fails the dashboard is meaningless and we should surface
	// the error rather than render zeros. Secondary counts are allowed to
	// degrade gracefully (logged but not returned) so a partial outage of one
	// downstream query doesn't blank the whole dashboard.
	statusCounts, err := s.q.CountLotsByStatusGroup(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("count lots by status: %w", err))
	}
	totalCount, _ := s.q.CountLots(ctx)
	awaitingQC, _ := s.q.CountLotsByStatus(ctx, db.LotsStatusQCREVIEW)
	ready, _ := s.q.CountLotsByStatus(ctx, db.LotsStatusREADYFORPRODUCTION)

	var protoStatuses []*dashv1.StatusCount
	for _, sc := range statusCounts {
		protoStatuses = append(protoStatuses, &dashv1.StatusCount{
			Status: string(sc.Status),
			Count:  int32(sc.Count),
		})
	}

	return connect.NewResponse(&dashv1.GetOpsDashboardResponse{
		LotsByStatus:           protoStatuses,
		TotalLots:              int32(totalCount),
		LotsAwaitingQc:         int32(awaitingQC),
		LotsReadyForProduction: int32(ready),
	}), nil
}

func (s *DashboardService) GetQCMetrics(ctx context.Context, req *connect.Request[dashv1.GetQCMetricsRequest]) (*connect.Response[dashv1.GetQCMetricsResponse], error) {
	hours := int32(24)
	if req.Msg.Hours > 0 {
		hours = req.Msg.Hours
	}
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	recCounts, _ := s.q.CountQCByRecommendation(ctx, since)
	avgConfRaw, _ := s.q.AvgQCConfidence(ctx, since)
	var avgConf float64
	switch v := avgConfRaw.(type) {
	case float64:
		avgConf = v
	case []byte:
		fmt.Sscanf(string(v), "%f", &avgConf)
	case string:
		fmt.Sscanf(v, "%f", &avgConf)
	}

	var pass, review, fail int32
	for _, rc := range recCounts {
		switch rc.Recommendation {
		case db.QcResultsRecommendationPASS:
			pass = int32(rc.Count)
		case db.QcResultsRecommendationREVIEW:
			review = int32(rc.Count)
		case db.QcResultsRecommendationFAIL:
			fail = int32(rc.Count)
		}
	}
	total := pass + review + fail
	var passRate float64
	if total > 0 {
		passRate = float64(pass) / float64(total)
	}

	pendingReview, _ := s.q.CountLotsByStatus(ctx, db.LotsStatusQCREVIEW)

	return connect.NewResponse(&dashv1.GetQCMetricsResponse{
		TotalJobs:          total,
		PassCount:          pass,
		ReviewCount:        review,
		FailCount:          fail,
		AverageConfidence:  avgConf,
		PassRate:           passRate,
		PendingReviewCount: int32(pendingReview),
	}), nil
}

func (s *DashboardService) GetWarehouseMetrics(ctx context.Context, req *connect.Request[dashv1.GetWarehouseMetricsRequest]) (*connect.Response[dashv1.GetWarehouseMetricsResponse], error) {
	zoneMetrics, _ := s.q.ZoneCapacityMetrics(ctx)

	var zones []*dashv1.ZoneMetrics
	var totalCap, totalOcc, totalAvail int32
	for _, zm := range zoneMetrics {
		cap := toInt32(zm.TotalCapacity)
		occ := toInt32(zm.Occupied)
		avail := toInt32(zm.Available)
		zones = append(zones, &dashv1.ZoneMetrics{
			Zone:          zm.Zone,
			TotalCapacity: cap,
			Occupied:      occ,
			Available:     avail,
		})
		totalCap += cap
		totalOcc += occ
		totalAvail += avail
	}

	return connect.NewResponse(&dashv1.GetWarehouseMetricsResponse{
		Zones:          zones,
		TotalCapacity:  totalCap,
		TotalOccupied:  totalOcc,
		TotalAvailable: totalAvail,
	}), nil
}

func toInt32(v interface{}) int32 {
	switch val := v.(type) {
	case int64:
		return int32(val)
	case int32:
		return val
	case []byte:
		var n int32
		fmt.Sscanf(string(val), "%d", &n)
		return n
	case string:
		var n int32
		fmt.Sscanf(val, "%d", &n)
		return n
	default:
		return 0
	}
}

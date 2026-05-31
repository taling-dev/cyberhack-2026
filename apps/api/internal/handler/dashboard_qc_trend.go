package handler

import (
	"context"
	"time"

	"connectrpc.com/connect"

	dashv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1"
)

// GetQCTrend returns per-day PASS/REVIEW/FAIL counts over the lookback window,
// with empty days zero-filled so the chart has a continuous axis.
func (s *DashboardService) GetQCTrend(ctx context.Context, req *connect.Request[dashv1.GetQCTrendRequest]) (*connect.Response[dashv1.GetQCTrendResponse], error) {
	days := int32(7)
	if req.Msg.Days > 0 {
		days = req.Msg.Days
	}
	since := time.Now().AddDate(0, 0, -int(days-1)).Truncate(24 * time.Hour)

	rows, err := s.q.QCTrendByDay(ctx, since)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	byDay := make(map[string]*dashv1.QCTrendDay, len(rows))
	for _, r := range rows {
		d := r.Day.Format("2006-01-02")
		byDay[d] = &dashv1.QCTrendDay{
			Date:        d,
			PassCount:   toInt32(r.PassCount),
			ReviewCount: toInt32(r.ReviewCount),
			FailCount:   toInt32(r.FailCount),
		}
	}

	out := make([]*dashv1.QCTrendDay, 0, days)
	for i := int32(0); i < days; i++ {
		d := since.AddDate(0, 0, int(i)).Format("2006-01-02")
		if v, ok := byDay[d]; ok {
			out = append(out, v)
		} else {
			out = append(out, &dashv1.QCTrendDay{Date: d})
		}
	}
	return connect.NewResponse(&dashv1.GetQCTrendResponse{Days: out}), nil
}

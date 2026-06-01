package handler

import (
	"context"
	"fmt"
	"strconv"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	adminv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1"
)

const (
	defaultQCPassMin   = 75
	defaultQCReviewMin = 40
	keyQCPassMin       = "qc_pass_min"
	keyQCReviewMin     = "qc_review_min"
)

func (s *AdminService) settingInt(ctx context.Context, key string, def int32) int32 {
	v, err := s.q.GetSetting(ctx, key)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return int32(n)
}

func (s *AdminService) GetQCThresholds(ctx context.Context, req *connect.Request[adminv1.GetQCThresholdsRequest]) (*connect.Response[adminv1.QCThresholds], error) {
	return connect.NewResponse(&adminv1.QCThresholds{
		PassMin:          s.settingInt(ctx, keyQCPassMin, defaultQCPassMin),
		ReviewMin:        s.settingInt(ctx, keyQCReviewMin, defaultQCReviewMin),
		DefaultPassMin:   defaultQCPassMin,
		DefaultReviewMin: defaultQCReviewMin,
	}), nil
}

func (s *AdminService) UpdateQCThresholds(ctx context.Context, req *connect.Request[adminv1.QCThresholds]) (*connect.Response[adminv1.QCThresholds], error) {
	pass, review := req.Msg.PassMin, req.Msg.ReviewMin
	// Validate: 0 < review_min < pass_min <= 100 so the PASS/REVIEW/FAIL bands
	// stay ordered and non-empty.
	if review < 1 || review >= pass || pass > 100 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("require 0 < review_min < pass_min <= 100"))
	}
	if err := s.q.UpsertSetting(ctx, db.UpsertSettingParams{SettingKey: keyQCPassMin, SettingValue: strconv.Itoa(int(pass))}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := s.q.UpsertSetting(ctx, db.UpsertSettingParams{SettingKey: keyQCReviewMin, SettingValue: strconv.Itoa(int(review))}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&adminv1.QCThresholds{
		PassMin: pass, ReviewMin: review,
		DefaultPassMin: defaultQCPassMin, DefaultReviewMin: defaultQCReviewMin,
	}), nil
}

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	dashv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1"
)

func recommendationToInt(r db.QcResultsRecommendation) int32 {
	switch r {
	case db.QcResultsRecommendationPASS:
		return 1
	case db.QcResultsRecommendationREVIEW:
		return 2
	case db.QcResultsRecommendationFAIL:
		return 3
	default:
		return 0
	}
}

// GetLatestInspection returns the most recent QC result across all lots, with
// the lot info + image key, so the dashboard can show a real "latest inspection"
// instead of a per-queue approximation. present=false when none exists yet.
func (s *DashboardService) GetLatestInspection(ctx context.Context, req *connect.Request[dashv1.GetLatestInspectionRequest]) (*connect.Response[dashv1.GetLatestInspectionResponse], error) {
	r, err := s.q.GetLatestInspection(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return connect.NewResponse(&dashv1.GetLatestInspectionResponse{Present: false}), nil
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := &dashv1.GetLatestInspectionResponse{
		Present:        true,
		LotId:          r.LotID,
		LotNumber:      r.LotNumber,
		MaterialName:   r.MaterialName,
		Recommendation: recommendationToInt(r.Recommendation),
		Confidence:     parseFloat(r.Confidence),
		ImageObjectKey: r.ImageObjectKey,
		CreatedAtUnix:  r.CreatedAt.Unix(),
	}

	if len(r.FindingsJson) > 0 {
		var findings []struct {
			MappedFinding string  `json:"mapped_finding"`
			ClassName     string  `json:"class_name"`
			Confidence    float64 `json:"confidence"`
			IsAnomaly     bool    `json:"is_anomaly"`
		}
		if json.Unmarshal(r.FindingsJson, &findings) == nil {
			for _, f := range findings {
				label := f.MappedFinding
				if label == "" {
					label = f.ClassName
				}
				resp.Findings = append(resp.Findings, &dashv1.InspectionFinding{
					MappedFinding: label,
					Confidence:    f.Confidence,
					IsAnomaly:     f.IsAnomaly,
				})
			}
		}
	}

	return connect.NewResponse(resp), nil
}

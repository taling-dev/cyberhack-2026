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
	qcv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

var _ qcv1connect.QCServiceHandler = (*QCService)(nil)

type QCService struct {
	q     *db.Queries
	minio *storage.MinIOClient
}

func NewQCService(q *db.Queries, minio *storage.MinIOClient) *QCService {
	return &QCService{q: q, minio: minio}
}

func (s *QCService) CreateQCUploadUrl(ctx context.Context, req *connect.Request[qcv1.CreateQCUploadUrlRequest]) (*connect.Response[qcv1.CreateQCUploadUrlResponse], error) {
	msg := req.Msg

	if msg.LotId == "" || msg.Filename == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and filename are required"))
	}

	contentType := msg.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ext := ".jpg"
	if contentType == "image/png" {
		ext = ".png"
	}

	objectKey := fmt.Sprintf("%s/%s%s", msg.LotId, uuid.NewString(), ext)
	expiry := 15 * time.Minute

	uploadURL, err := s.minio.PresignedPutURL(ctx, storage.QCImagesBucket, objectKey, contentType, expiry)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to generate upload URL: %w", err))
	}

	expiresAt := time.Now().Add(expiry).Unix()

	return connect.NewResponse(&qcv1.CreateQCUploadUrlResponse{
		ObjectKey:    objectKey,
		UploadUrl:    uploadURL,
		ExpiresAtUnix: expiresAt,
	}), nil
}

func (s *QCService) CreateQCJob(ctx context.Context, req *connect.Request[qcv1.CreateQCJobRequest]) (*connect.Response[qcv1.CreateQCJobResponse], error) {
	msg := req.Msg
	if msg.LotId == "" || msg.ImageObjectKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and image_object_key required"))
	}

	jobID := uuid.NewString()

	// Create QC job
	err := s.q.CreateQCJob(ctx, db.CreateQCJobParams{
		ID:             jobID,
		LotID:          msg.LotId,
		ImageObjectKey: msg.ImageObjectKey,
		RequestedBy:    "dev-user", // TODO: from JWT in Task 15
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create qc job: %w", err))
	}

	// Advance lot to PENDING_QC
	_ = s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusPENDINGQC, ID: msg.LotId})

	// ─── Inline mock AI (removed in Task 19 when NATS worker takes over) ───
	resultID := uuid.NewString()
	findingsJSON := []byte(`[{"class_name":"bottle","mapped_finding":"foreign_matter","confidence":0.87,"is_anomaly":true},{"class_name":"banana","mapped_finding":"ripeness_signal","confidence":0.92,"is_anomaly":false}]`)

	err = s.q.CreateQCResult(ctx, db.CreateQCResultParams{
		ID:             resultID,
		QcJobID:        jobID,
		LotID:          msg.LotId,
		Recommendation: db.QcResultsRecommendationREVIEW,
		Confidence:     "0.8200",
		FindingsJson:   findingsJSON,
		ModelVersion:   "mock-v0.1.0",
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create qc result: %w", err))
	}

	// Advance job to AI_COMPLETED and lot to QC_REVIEW
	_ = s.q.UpdateQCJobCompleted(ctx, db.UpdateQCJobCompletedParams{Status: db.QcJobsStatusAICOMPLETED, ID: jobID})
	_ = s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusQCREVIEW, ID: msg.LotId})

	job, _ := s.q.GetQCJob(ctx, jobID)
	return connect.NewResponse(&qcv1.CreateQCJobResponse{Job: dbJobToProto(job)}), nil
}

func (s *QCService) GetQCJob(ctx context.Context, req *connect.Request[qcv1.GetQCJobRequest]) (*connect.Response[qcv1.GetQCJobResponse], error) {
	job, err := s.q.GetQCJob(ctx, req.Msg.QcJobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("qc job not found"))
	}
	return connect.NewResponse(&qcv1.GetQCJobResponse{Job: dbJobToProto(job)}), nil
}

func (s *QCService) GetQCResult(ctx context.Context, req *connect.Request[qcv1.GetQCResultRequest]) (*connect.Response[qcv1.GetQCResultResponse], error) {
	result, err := s.q.GetQCResult(ctx, req.Msg.QcJobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("qc result not found"))
	}
	return connect.NewResponse(&qcv1.GetQCResultResponse{Result: dbResultToProto(result)}), nil
}

func (s *QCService) ReviewQC(ctx context.Context, req *connect.Request[qcv1.ReviewQCRequest]) (*connect.Response[qcv1.ReviewQCResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 11"))
}

func (s *QCService) RetryQCJob(ctx context.Context, req *connect.Request[qcv1.RetryQCJobRequest]) (*connect.Response[qcv1.RetryQCJobResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 19"))
}

// ─── Helpers ─────────────────────────────────────────────────────

func dbJobToProto(j db.QcJob) *qcv1.QCJob {
	pj := &qcv1.QCJob{
		Id:             j.ID,
		LotId:          j.LotID,
		ImageObjectKey: j.ImageObjectKey,
		Status:         qcJobStatusFromDB(string(j.Status)),
		RequestedBy:    j.RequestedBy,
	}
	if j.StartedAt.Valid {
		pj.StartedAt = timestamppb.New(j.StartedAt.Time)
	}
	if j.CompletedAt.Valid {
		pj.CompletedAt = timestamppb.New(j.CompletedAt.Time)
	}
	pj.CreatedAt = timestamppb.New(j.CreatedAt)
	pj.UpdatedAt = timestamppb.New(j.UpdatedAt)
	return pj
}

func qcJobStatusFromDB(s string) qcv1.QCJobStatus {
	switch s {
	case "QUEUED":
		return qcv1.QCJobStatus_QC_JOB_STATUS_QUEUED
	case "PROCESSING":
		return qcv1.QCJobStatus_QC_JOB_STATUS_PROCESSING
	case "AI_COMPLETED":
		return qcv1.QCJobStatus_QC_JOB_STATUS_AI_COMPLETED
	case "NEEDS_HUMAN_REVIEW":
		return qcv1.QCJobStatus_QC_JOB_STATUS_NEEDS_HUMAN_REVIEW
	case "APPROVED":
		return qcv1.QCJobStatus_QC_JOB_STATUS_APPROVED
	case "REJECTED":
		return qcv1.QCJobStatus_QC_JOB_STATUS_REJECTED
	case "FAILED":
		return qcv1.QCJobStatus_QC_JOB_STATUS_FAILED
	default:
		return qcv1.QCJobStatus_QC_JOB_STATUS_UNSPECIFIED
	}
}

func dbResultToProto(r db.QcResult) *qcv1.QCResult {
	pr := &qcv1.QCResult{
		Id:             r.ID,
		QcJobId:        r.QcJobID,
		LotId:          r.LotID,
		Recommendation: qcRecommendationFromDB(string(r.Recommendation)),
		Confidence:     parseFloat(r.Confidence),
		ModelVersion:   r.ModelVersion,
		CreatedAt:      timestamppb.New(r.CreatedAt),
	}

	// Parse findings JSON
	if len(r.FindingsJson) > 0 {
		var findings []struct {
			ClassName     string  `json:"class_name"`
			MappedFinding string  `json:"mapped_finding"`
			Confidence    float64 `json:"confidence"`
			IsAnomaly     bool    `json:"is_anomaly"`
		}
		if json.Unmarshal(r.FindingsJson, &findings) == nil {
			for _, f := range findings {
				pr.Findings = append(pr.Findings, &qcv1.QCFinding{
					ClassName:     f.ClassName,
					MappedFinding: f.MappedFinding,
					Confidence:    f.Confidence,
					IsAnomaly:     f.IsAnomaly,
				})
			}
		}
	}

	if r.AnnotatedImageKey.Valid {
		pr.AnnotatedImageKey = r.AnnotatedImageKey.String
	}
	if r.SupervisorDecision.Valid {
		pr.SupervisorDecision = supervisorDecisionFromDB(string(r.SupervisorDecision.QcResultsSupervisorDecision))
	}
	if r.ReviewedBy.Valid {
		pr.ReviewedBy = r.ReviewedBy.String
	}
	if r.ReviewReason.Valid {
		pr.ReviewReason = r.ReviewReason.String
	}
	if r.ReviewedAt.Valid {
		pr.ReviewedAt = timestamppb.New(r.ReviewedAt.Time)
	}
	return pr
}

func qcRecommendationFromDB(s string) qcv1.QCRecommendation {
	switch s {
	case "PASS":
		return qcv1.QCRecommendation_QC_RECOMMENDATION_PASS
	case "REVIEW":
		return qcv1.QCRecommendation_QC_RECOMMENDATION_REVIEW
	case "FAIL":
		return qcv1.QCRecommendation_QC_RECOMMENDATION_FAIL
	default:
		return qcv1.QCRecommendation_QC_RECOMMENDATION_UNSPECIFIED
	}
}

func supervisorDecisionFromDB(s string) qcv1.SupervisorDecision {
	switch s {
	case "APPROVED":
		return qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED
	case "REJECTED":
		return qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED
	case "RECHECK":
		return qcv1.SupervisorDecision_SUPERVISOR_DECISION_RECHECK
	default:
		return qcv1.SupervisorDecision_SUPERVISOR_DECISION_UNSPECIFIED
	}
}

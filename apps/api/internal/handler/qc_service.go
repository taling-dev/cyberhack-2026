package handler

import (
	"context"
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
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 10"))
}

func (s *QCService) GetQCResult(ctx context.Context, req *connect.Request[qcv1.GetQCResultRequest]) (*connect.Response[qcv1.GetQCResultResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("implemented in Task 10"))
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

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	qcv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
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

func (s *QCService) CreateQCViewUrl(ctx context.Context, req *connect.Request[qcv1.CreateQCViewUrlRequest]) (*connect.Response[qcv1.CreateQCViewUrlResponse], error) {
	if req.Msg.ObjectKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("object_key required"))
	}
	expiry := 15 * time.Minute
	url, err := s.minio.PresignedGetURL(ctx, storage.QCImagesBucket, req.Msg.ObjectKey, expiry)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("presign get: %w", err))
	}
	return connect.NewResponse(&qcv1.CreateQCViewUrlResponse{
		ViewUrl:       url,
		ExpiresAtUnix: time.Now().Add(expiry).Unix(),
	}), nil
}

func (s *QCService) CreateQCJob(ctx context.Context, req *connect.Request[qcv1.CreateQCJobRequest]) (*connect.Response[qcv1.CreateQCJobResponse], error) {
	msg := req.Msg
	if msg.LotId == "" || msg.ImageObjectKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and image_object_key required"))
	}

	// Fetch lot to get material_type for the AI worker
	lot, err := s.q.GetLot(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}

	jobID := uuid.NewString()

	// Create QC job
	err = s.q.CreateQCJob(ctx, db.CreateQCJobParams{
		ID:             jobID,
		LotID:          msg.LotId,
		ImageObjectKey: msg.ImageObjectKey,
		RequestedBy:    userFromCtx(ctx),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create qc job: %w", err))
	}

	// Advance lot to PENDING_QC (don't ignore error)
	if err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusPENDINGQC, ID: msg.LotId}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	// Write outbox event for async processing
	outboxPayload, _ := json.Marshal(map[string]string{
		"qc_job_id":        jobID,
		"lot_id":           msg.LotId,
		"image_object_key": msg.ImageObjectKey,
		"material_type":    string(lot.MaterialType),
	})
	if err := s.q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "qc.job.created",
		PayloadJson: outboxPayload,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	job, _ := s.q.GetQCJob(ctx, jobID)
	return connect.NewResponse(&qcv1.CreateQCJobResponse{Job: dbJobToProto(job)}), nil
}

func (s *QCService) GetQCJob(ctx context.Context, req *connect.Request[qcv1.GetQCJobRequest]) (*connect.Response[qcv1.GetQCJobResponse], error) {
	// If lot_id is provided, find the latest QC job for that lot
	if req.Msg.LotId != "" && req.Msg.QcJobId == "" {
		jobs, err := s.q.ListQCJobsByLot(ctx, req.Msg.LotId)
		if err != nil || len(jobs) == 0 {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no qc jobs for lot"))
		}
		// Latest first (sql sorts by created_at DESC)
		return connect.NewResponse(&qcv1.GetQCJobResponse{Job: dbJobToProto(jobs[0])}), nil
	}

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
	msg := req.Msg
	if msg.QcJobId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("qc_job_id required"))
	}
	if msg.Decision == qcv1.SupervisorDecision_SUPERVISOR_DECISION_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("decision required"))
	}

	// Fetch job to validate state
	job, err := s.q.GetQCJob(ctx, msg.QcJobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("qc job not found"))
	}
	if job.Status != db.QcJobsStatusAICOMPLETED && job.Status != db.QcJobsStatusNEEDSHUMANREVIEW {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("job not in reviewable state (current: %s)", job.Status))
	}

	// Fetch result to check if reason is required (override)
	result, err := s.q.GetQCResult(ctx, msg.QcJobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("qc result not found for job"))
	}

	// Reason enforcement: required on rejection, and on approval when AI recommended REVIEW or FAIL
	isOverride := (msg.Decision == qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED &&
		(result.Recommendation == db.QcResultsRecommendationREVIEW || result.Recommendation == db.QcResultsRecommendationFAIL))
	isRejection := msg.Decision == qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED

	if (isOverride || isRejection) && msg.Reason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("reason is required for rejections and approval overrides"))
	}

	// Map decision to DB enum
	var dbDecision db.NullQcResultsSupervisorDecision
	switch msg.Decision {
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED:
		dbDecision = db.NullQcResultsSupervisorDecision{QcResultsSupervisorDecision: "APPROVED", Valid: true}
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED:
		dbDecision = db.NullQcResultsSupervisorDecision{QcResultsSupervisorDecision: "REJECTED", Valid: true}
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_RECHECK:
		dbDecision = db.NullQcResultsSupervisorDecision{QcResultsSupervisorDecision: "RECHECK", Valid: true}
	}

	// Update QC result with supervisor decision
	reviewer := userFromCtx(ctx)
	err = s.q.UpdateQCResultReview(ctx, db.UpdateQCResultReviewParams{
		SupervisorDecision: dbDecision,
		ReviewedBy:         toNullString(reviewer),
		ReviewReason:       toNullString(msg.Reason),
		QcJobID:            msg.QcJobId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update review: %w", err))
	}

	// Advance lot and job status based on decision (propagate errors)
	switch msg.Decision {
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED:
		if err := s.q.UpdateQCJobCompleted(ctx, db.UpdateQCJobCompletedParams{Status: db.QcJobsStatusAPPROVED, ID: msg.QcJobId}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update qc job: %w", err))
		}
		if err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusQCAPPROVED, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED:
		if err := s.q.UpdateQCJobCompleted(ctx, db.UpdateQCJobCompletedParams{Status: db.QcJobsStatusREJECTED, ID: msg.QcJobId}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update qc job: %w", err))
		}
		if err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusQCREJECTED, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_RECHECK:
		if err := s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusPENDINGQC, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
	}

	now := time.Now()

	// Metrics
	switch msg.Decision {
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED:
		middleware.IncQCReviewed("approved")
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED:
		middleware.IncQCReviewed("rejected")
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_RECHECK:
		middleware.IncQCReviewed("recheck")
	}

	return connect.NewResponse(&qcv1.ReviewQCResponse{
		QcJobId:    msg.QcJobId,
		Decision:   msg.Decision,
		ReviewedBy: reviewer,
		ReviewedAt: timestamppb.New(now),
	}), nil
}

func (s *QCService) RetryQCJob(ctx context.Context, req *connect.Request[qcv1.RetryQCJobRequest]) (*connect.Response[qcv1.RetryQCJobResponse], error) {
	if req.Msg.QcJobId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("qc_job_id required"))
	}

	// Fetch job
	job, err := s.q.GetQCJob(ctx, req.Msg.QcJobId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("qc job not found"))
	}

	// Only FAILED jobs can be retried
	if job.Status != db.QcJobsStatusFAILED {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("only FAILED jobs can be retried (current: %s)", job.Status))
	}

	// Reset job to QUEUED, clear failure_reason
	if err := s.q.UpdateQCJobStatus(ctx, db.UpdateQCJobStatusParams{
		Status: db.QcJobsStatusQUEUED,
		ID:     job.ID,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("reset job: %w", err))
	}

	// Re-fetch lot for material_type
	lot, err := s.q.GetLot(ctx, job.LotID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("lot lookup: %w", err))
	}

	// Re-publish via outbox so the AI worker picks it up again
	outboxPayload, _ := json.Marshal(map[string]string{
		"qc_job_id":        job.ID,
		"lot_id":           job.LotID,
		"image_object_key": job.ImageObjectKey,
		"material_type":    string(lot.MaterialType),
	})
	if err := s.q.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "qc.job.created",
		PayloadJson: outboxPayload,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	// Lot back to PENDING_QC
	_ = s.q.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatusPENDINGQC,
		ID:     job.LotID,
	})

	freshJob, _ := s.q.GetQCJob(ctx, job.ID)
	return connect.NewResponse(&qcv1.RetryQCJobResponse{
		Job: dbJobToProto(freshJob),
	}), nil
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

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

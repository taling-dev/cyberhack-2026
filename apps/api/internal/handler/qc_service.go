package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	qcv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/middleware"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

// allowedQCImageContentTypes restricts uploads to image MIME types we know
// browsers render safely and that won't double as XSS vectors. SVG is
// deliberately excluded — it's an XSS sink. HTML/JS/etc. are excluded by
// virtue of not being on the list.
var allowedQCImageContentTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var _ qcv1connect.QCServiceHandler = (*QCService)(nil)

type QCService struct {
	q     *db.Queries
	dbx   *sql.DB
	minio *storage.MinIOClient
}

func NewQCService(q *db.Queries, dbx *sql.DB, minio *storage.MinIOClient) *QCService {
	return &QCService{q: q, dbx: dbx, minio: minio}
}

func (s *QCService) CreateQCUploadUrl(ctx context.Context, req *connect.Request[qcv1.CreateQCUploadUrlRequest]) (*connect.Response[qcv1.CreateQCUploadUrlResponse], error) {
	if s.minio == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("object storage unavailable"))
	}
	msg := req.Msg

	if msg.LotId == "" || msg.Filename == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("lot_id and filename are required"))
	}

	contentType := msg.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	ext, ok := allowedQCImageContentTypes[contentType]
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("content_type must be one of: image/jpeg, image/png, image/webp"))
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

// CreateQCViewUrl mints a short-lived presigned GET for a QC image. We
// authorize the caller against the lot referenced by the object key prefix:
//   - OPERATOR: only lots they created (owner check)
//   - QC_SUPERVISOR / MANAGER / ADMIN / WAREHOUSE_STAFF: any lot
//
// The object key format is `<lot_id>/<uuid>.<ext>`; we extract the lot_id
// prefix and load the lot to enforce ownership. This closes an IDOR where
// any authenticated user could view any QC image by guessing object keys.
func (s *QCService) CreateQCViewUrl(ctx context.Context, req *connect.Request[qcv1.CreateQCViewUrlRequest]) (*connect.Response[qcv1.CreateQCViewUrlResponse], error) {
	if s.minio == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("object storage unavailable"))
	}
	if req.Msg.ObjectKey == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("object_key required"))
	}

	// Parse and validate the lot_id prefix. uuid.Parse rejects directory
	// traversal attempts (`../`) and arbitrary keys outside our schema.
	parts := strings.SplitN(req.Msg.ObjectKey, "/", 2)
	if len(parts) != 2 || parts[1] == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("malformed object_key"))
	}
	lotID := parts[0]
	if _, err := uuid.Parse(lotID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("malformed object_key: %w", err))
	}

	lot, err := s.q.GetLot(ctx, lotID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}

	// Owner check for OPERATOR. roleFromCtx returns the caller's primary role;
	// userFromCtx returns the username (the value services write to created_by).
	role := roleFromCtx(ctx)
	if role == "OPERATOR" {
		caller := userFromCtx(ctx)
		if lot.CreatedBy != caller {
			return nil, connect.NewError(connect.CodePermissionDenied,
				fmt.Errorf("operators may only view images for lots they created"))
		}
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

	// Fetch lot to get material_type for the AI worker (read-only, no tx needed)
	lot, err := s.q.GetLot(ctx, msg.LotId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("lot not found"))
	}

	jobID := uuid.NewString()

	// Begin transaction — wraps job insert, lot status update, and outbox event.
	// If any step fails, all roll back atomically: no orphan jobs, no advanced lots
	// without a corresponding outbox event for the AI worker to consume.
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }() // no-op if Commit succeeded

	qtx := s.q.WithTx(tx)

	// Reject a second QC job while one is already in flight for this lot, so a
	// double-click (or re-upload during PENDING_QC) can't spawn duplicate jobs.
	if active, err := qtx.CountActiveQCJobsForLot(ctx, msg.LotId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("count active qc jobs: %w", err))
	} else if active > 0 {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("lot already has an active QC job"))
	}

	if err := qtx.CreateQCJob(ctx, db.CreateQCJobParams{
		ID:             jobID,
		LotID:          msg.LotId,
		ImageObjectKey: msg.ImageObjectKey,
		RequestedBy:    userFromCtx(ctx),
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create qc job: %w", err))
	}

	if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusPENDINGQC, ID: msg.LotId}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	outboxPayload, _ := json.Marshal(map[string]string{
		"qc_job_id":        jobID,
		"lot_id":           msg.LotId,
		"image_object_key": msg.ImageObjectKey,
		"material_type":    string(lot.MaterialType),
		"owner_user_id":    lot.CreatedBy,
	})
	envelope, err := events.NewEnvelope(
		"qc.job.created",
		userFromCtx(ctx),
		lot.CreatedBy, // owner = lot creator, not the QC requester
		jobID,
		json.RawMessage(outboxPayload),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "qc.job.created",
		PayloadJson: envelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
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

	// Fetch lot once so we have created_by for envelope owner_user_id and the
	// lot_number for downstream toasts.
	lot, lotErr := s.q.GetLot(ctx, job.LotID)
	if lotErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("lot lookup: %w", lotErr))
	}

	// Wrap result update + qc_job/lot status advance in a single tx.
	// All four mutations (qc_results.review, qc_jobs.status, lots.status [×2 paths])
	// must succeed or fail together so the lot/job state never drifts.
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	// Update QC result with supervisor decision
	reviewer := userFromCtx(ctx)
	err = qtx.UpdateQCResultReview(ctx, db.UpdateQCResultReviewParams{
		SupervisorDecision: dbDecision,
		ReviewedBy:         toNullString(reviewer),
		ReviewReason:       toNullString(msg.Reason),
		QcJobID:            msg.QcJobId,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update review: %w", err))
	}

	// Advance lot and job status based on decision (propagate errors)
	var lotStatusAfter string
	switch msg.Decision {
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED:
		if err := qtx.UpdateQCJobCompleted(ctx, db.UpdateQCJobCompletedParams{Status: db.QcJobsStatusAPPROVED, ID: msg.QcJobId}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update qc job: %w", err))
		}
		if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusQCAPPROVED, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
		lotStatusAfter = "QC_APPROVED"
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_REJECTED:
		if err := qtx.UpdateQCJobCompleted(ctx, db.UpdateQCJobCompletedParams{Status: db.QcJobsStatusREJECTED, ID: msg.QcJobId}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update qc job: %w", err))
		}
		if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusQCREJECTED, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
		lotStatusAfter = "QC_REJECTED"
	case qcv1.SupervisorDecision_SUPERVISOR_DECISION_RECHECK:
		if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{Status: db.LotsStatusPENDINGQC, ID: job.LotID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
		}
		lotStatusAfter = "PENDING_QC"
	}

	// Emit qc.job.reviewed event so the SSE bridge can fan out to QC supervisors
	// (table refresh) and warehouse staff (becomes actionable on approval).
	reviewEnvelope, err := events.NewEnvelope(
		"qc.job.reviewed",
		reviewer,
		lot.CreatedBy,
		msg.QcJobId,
		map[string]any{
			"qc_job_id":        msg.QcJobId,
			"lot_id":           job.LotID,
			"lot_number":       lot.LotNumber,
			"lot_created_by":   lot.CreatedBy,
			"decision":         msg.Decision.String(),
			"reason":           msg.Reason,
			"reviewer":         reviewer,
			"lot_status_after": lotStatusAfter,
		},
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build review envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "qc.job.reviewed",
		PayloadJson: reviewEnvelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create review outbox event: %w", err))
	}

	// On approval, also emit lot.status_changed so warehouse staff get a
	// "ready to slot" toast without scraping qc.* subjects.
	if msg.Decision == qcv1.SupervisorDecision_SUPERVISOR_DECISION_APPROVED {
		statusEnvelope, err := events.NewEnvelope(
			"lot.status_changed",
			reviewer,
			lot.CreatedBy,
			job.LotID,
			map[string]any{
				"lot_id":     job.LotID,
				"lot_number": lot.LotNumber,
				"from":       "QC_REVIEW",
				"to":         "QC_APPROVED",
				"reason":     "qc-approved",
				"created_by": lot.CreatedBy,
				"actor_id":   reviewer,
			},
		)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build status envelope: %w", err))
		}
		if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
			ID:          uuid.NewString(),
			EventType:   "lot.status_changed",
			PayloadJson: statusEnvelope,
		}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create lot status outbox event: %w", err))
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
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

	// Re-fetch lot for material_type
	lot, err := s.q.GetLot(ctx, job.LotID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("lot lookup: %w", err))
	}

	// Wrap retry mutations in tx: job reset + outbox event + lot rollback to PENDING_QC
	tx, err := s.dbx.BeginTx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.q.WithTx(tx)

	if err := qtx.UpdateQCJobStatus(ctx, db.UpdateQCJobStatusParams{
		Status: db.QcJobsStatusQUEUED,
		ID:     job.ID,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("reset job: %w", err))
	}

	outboxPayload, _ := json.Marshal(map[string]string{
		"qc_job_id":        job.ID,
		"lot_id":           job.LotID,
		"image_object_key": job.ImageObjectKey,
		"material_type":    string(lot.MaterialType),
		"owner_user_id":    lot.CreatedBy,
	})
	retryEnvelope, err := events.NewEnvelope(
		"qc.job.created",
		userFromCtx(ctx),
		lot.CreatedBy,
		job.ID,
		json.RawMessage(outboxPayload),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("build envelope: %w", err))
	}
	if err := qtx.CreateOutboxEvent(ctx, db.CreateOutboxEventParams{
		ID:          uuid.NewString(),
		EventType:   "qc.job.created",
		PayloadJson: retryEnvelope,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create outbox event: %w", err))
	}

	if err := qtx.UpdateLotStatus(ctx, db.UpdateLotStatusParams{
		Status: db.LotsStatusPENDINGQC,
		ID:     job.LotID,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update lot status: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("commit: %w", err))
	}

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

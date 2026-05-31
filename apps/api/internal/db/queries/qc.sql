-- name: CreateQCJob :exec
INSERT INTO qc_jobs (id, lot_id, image_object_key, status, requested_by)
VALUES (?, ?, ?, 'QUEUED', ?);

-- name: GetQCJob :one
SELECT * FROM qc_jobs WHERE id = ?;

-- name: ListQCJobsByLot :many
SELECT * FROM qc_jobs WHERE lot_id = ? ORDER BY created_at DESC;

-- name: CountActiveQCJobsForLot :one
-- Active = not yet in a terminal state (APPROVED/REJECTED/FAILED). Used to
-- reject a duplicate CreateQCJob while a job for the lot is still in flight.
SELECT COUNT(*) FROM qc_jobs
WHERE lot_id = ? AND status IN ('QUEUED','PROCESSING','AI_COMPLETED','NEEDS_HUMAN_REVIEW');

-- name: CountActiveQCJobsForLotWithImage :one
-- Same-image double-click guard: an active job already exists for this lot
-- AND the same image. Re-submitting the identical image is a no-op we reject;
-- a DIFFERENT image is a deliberate re-upload that supersedes the old job.
SELECT COUNT(*) FROM qc_jobs
WHERE lot_id = ? AND image_object_key = ?
  AND status IN ('QUEUED','PROCESSING','AI_COMPLETED','NEEDS_HUMAN_REVIEW');

-- name: SupersedeActiveQCJobsForLot :exec
-- Terminate any in-flight jobs for a lot (used when a new image is uploaded,
-- so the stale job/result is retired). FAILED is terminal and excluded from
-- the dashboard recommendation breakdown, so it doesn't skew metrics.
UPDATE qc_jobs SET status='FAILED', failure_reason='superseded by new image', completed_at=NOW()
WHERE lot_id = ? AND status IN ('QUEUED','PROCESSING','AI_COMPLETED','NEEDS_HUMAN_REVIEW');

-- name: RequeueQCJob :exec
-- RECHECK: re-queue an existing job so the worker re-runs AI on the SAME image.
UPDATE qc_jobs SET status='QUEUED', started_at=NULL, completed_at=NULL, failure_reason=NULL
WHERE id = ?;

-- name: ListQCJobsByStatus :many
SELECT * FROM qc_jobs WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?;

-- name: UpdateQCJobStatus :exec
UPDATE qc_jobs SET status = ? WHERE id = ?;

-- name: UpdateQCJobStarted :exec
UPDATE qc_jobs SET status = 'PROCESSING', started_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateQCJobCompleted :exec
UPDATE qc_jobs SET status = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateQCJobFailed :exec
UPDATE qc_jobs SET status = 'FAILED', failure_reason = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: CreateQCResult :exec
INSERT INTO qc_results (id, qc_job_id, lot_id, recommendation, confidence, findings_json, model_version, annotated_image_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetQCResult :one
SELECT * FROM qc_results WHERE qc_job_id = ?;

-- name: UpdateQCResultReview :exec
UPDATE qc_results SET supervisor_decision = ?, reviewed_by = ?, review_reason = ?, reviewed_at = CURRENT_TIMESTAMP WHERE qc_job_id = ?;

-- name: CountQCByRecommendation :many
SELECT recommendation, COUNT(*) as count FROM qc_results WHERE created_at >= ? GROUP BY recommendation;

-- name: AvgQCConfidence :one
SELECT COALESCE(AVG(confidence), 0) FROM qc_results WHERE created_at >= ?;

-- name: QCTrendByDay :many
SELECT DATE(created_at) AS day,
       SUM(recommendation = 'PASS')   AS pass_count,
       SUM(recommendation = 'REVIEW') AS review_count,
       SUM(recommendation = 'FAIL')   AS fail_count
FROM qc_results
WHERE created_at >= ?
GROUP BY DATE(created_at)
ORDER BY day;

-- name: LatestQCResultsForLots :many
SELECT r.lot_id, r.recommendation, r.confidence
FROM qc_results r
JOIN (
  SELECT q.lot_id AS lid, MAX(q.created_at) AS max_created
  FROM qc_results q
  WHERE q.lot_id IN (sqlc.slice('lot_ids'))
  GROUP BY q.lot_id
) latest ON latest.lid = r.lot_id AND latest.max_created = r.created_at;

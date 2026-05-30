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

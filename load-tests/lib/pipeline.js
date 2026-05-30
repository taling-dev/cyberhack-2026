// lib/pipeline.js
//
// One full end-to-end pipeline iteration. Used by every scenario:
//   * smoke — runs once-per-second-ish for a minute
//   * validation — runs flat-out from each VU
//   * soak — runs at constant arrival rate from a VU pool
//
// Steps (each tagged with its own rpc:* label so k6 thresholds can target
// individual operations, and the AI-blocking step is excluded from the
// "non_ai" composite):
//
//   1. CreateLot (operator)
//   2. CreateQCUploadUrl (operator)
//   3. PUT image → MinIO presigned URL
//   4. CreateQCJob (operator)               → lot moves to PENDING_QC
//   5. Poll GetLot every 1s for ≤ 15s      → AI worker advances to QC_REVIEW
//   6. ReviewQC (supervisor, decision=APPROVED)
//   7. RecommendSlot + AssignSlot (warehouse) → lot READY_FOR_PRODUCTION
//   8. CreateDispatch + UpdateDispatchStatus (warehouse) → ship the lot
//   9. GetLotTimeline (admin) → assert ≥ 3 audit entries
//
// Emits:
//   * `pipeline_e2e_completed{result=success|failed}` Counter
//   * `pipeline_e2e_duration` Trend (millis end-to-end)
//   * `ai_processing_duration` Trend (poll start → status=QC_REVIEW)

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

import {
  API,
  getOperatorToken,
  getSupervisorToken,
  getWarehouseToken,
  getAdminToken,
} from './auth.js';
import { randomLotData, uuid, qcImageBytes } from './helpers.js';

export const pipelineCompleted = new Counter('pipeline_e2e_completed');
export const pipelineDuration  = new Trend('pipeline_e2e_duration', true);
export const aiProcessingDuration = new Trend('ai_processing_duration', true);

// Common Connect-RPC headers. Bearer is added per-call so token rotation
// still works mid-iteration without restarting the run.
function headers(token, idempotencyKey, extraTags = {}) {
  const h = {
    'Content-Type': 'application/json',
    'Connect-Protocol-Version': '1',
    Authorization: `Bearer ${token}`,
  };
  if (idempotencyKey) h['Idempotency-Key'] = idempotencyKey;
  return h;
}

// rpc helper — wraps http.post with the standard tags. Returns the parsed
// JSON body or null on non-2xx (the caller handles failures).
function rpc(token, path, body, rpcTag, extraTags = {}, idemKey = null) {
  const res = http.post(`${API}${path}`, JSON.stringify(body), {
    headers: headers(token, idemKey),
    tags: { rpc: rpcTag, rpc_class: rpcTag === 'get_lot_poll' ? 'ai' : 'non_ai', ...extraTags },
  });
  // The Connect protocol uses HTTP 200 + a structured error body for RPC
  // failures (e.g., {"code":"failed_precondition", ...}). 4xx means a
  // transport-level failure (auth, body validation). Both are failures
  // from the test's perspective.
  const ok = res.status >= 200 && res.status < 300;
  if (!ok) {
    return { ok: false, status: res.status, body: res.body, json: safeJson(res) };
  }
  return { ok: true, status: res.status, json: safeJson(res), raw: res };
}

function safeJson(res) {
  try { return res.json(); } catch { return null; }
}

// runPipeline — performs one full E2E iteration. Returns true on success,
// false on any step failure (caller increments the counters; we already do).
export function runPipeline() {
  const t0 = Date.now();

  const opTok  = getOperatorToken();
  const supTok = getSupervisorToken();
  const whTok  = getWarehouseToken();
  const admTok = getAdminToken();

  // 1. CreateLot
  const lotReq = randomLotData(__VU, __ITER);
  const r1 = rpc(opTok, '/simaops.lot.v1.LotService/CreateLot', lotReq, 'create_lot');
  if (!check(r1, { 'create lot ok': (r) => r.ok })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'create_lot' });
    return false;
  }
  const lotId = r1.json && r1.json.lot && r1.json.lot.id;
  if (!lotId) {
    pipelineCompleted.add(1, { result: 'failed', step: 'create_lot_no_id' });
    return false;
  }

  // 2. CreateQCUploadUrl
  const r2 = rpc(opTok, '/simaops.qc.v1.QCService/CreateQCUploadUrl', {
    lotId,
    filename: 'qc.jpg',
    contentType: 'image/jpeg',
    idempotencyKey: `up-${uuid().slice(0, 8)}`,
  }, 'create_qc_upload_url');
  if (!check(r2, { 'upload url ok': (r) => r.ok && r.json && r.json.uploadUrl })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'upload_url' });
    return false;
  }
  const objectKey = r2.json.objectKey;

  // 3. PUT image to MinIO via presigned URL.
  const putRes = http.put(r2.json.uploadUrl, qcImageBytes(), {
    headers: { 'Content-Type': 'image/jpeg' },
    tags: { rpc: 'minio_upload', rpc_class: 'non_ai' },
  });
  if (!check(putRes, { 'minio put 200': (r) => r.status === 200 })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'minio_upload' });
    return false;
  }

  // 4. CreateQCJob — moves the lot to PENDING_QC and produces a NATS
  //    qc.job.created event that the AI worker pulls.
  const r4 = rpc(opTok, '/simaops.qc.v1.QCService/CreateQCJob', {
    lotId,
    imageObjectKey: objectKey,
    idempotencyKey: `qc-${uuid().slice(0, 8)}`,
  }, 'create_qc_job');
  if (!check(r4, { 'create qc job ok': (r) => r.ok })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'create_qc_job' });
    return false;
  }
  const qcJobId = r4.json && r4.json.job && r4.json.job.id;

  // 5. Poll GetLot until status reaches QC_REVIEW (mock AI takes ~1s; we
  //    bound at 15s for the validation worst-case where queue lag is high).
  const aiStart = Date.now();
  let aiCompleted = false;
  for (let i = 0; i < 15; i++) {
    sleep(1);
    const rPoll = rpc(opTok, '/simaops.lot.v1.LotService/GetLot', { lotId }, 'get_lot_poll');
    if (rPoll.ok && rPoll.json && rPoll.json.lot && rPoll.json.lot.status === 'LOT_STATUS_QC_REVIEW') {
      aiCompleted = true;
      break;
    }
  }
  aiProcessingDuration.add(Date.now() - aiStart);
  if (!aiCompleted) {
    pipelineCompleted.add(1, { result: 'failed', step: 'ai_timeout' });
    return false;
  }

  // 6. ReviewQC as supervisor, decision = APPROVED (1).
  const r6 = rpc(supTok, '/simaops.qc.v1.QCService/ReviewQC', {
    qcJobId,
    decision: 1,
    reason: 'load test auto-approve',
    idempotencyKey: `rev-${uuid().slice(0, 8)}`,
  }, 'review_qc');
  if (!check(r6, { 'review qc ok': (r) => r.ok })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'review_qc' });
    return false;
  }

  // 7a. RecommendSlot — warehouse picks the first compatible slot.
  const r7a = rpc(whTok, '/simaops.warehouse.v1.WarehouseService/RecommendSlot',
    { lotId }, 'recommend_slot');
  if (!check(r7a, {
    'recommend slot ok': (r) => r.ok && r.json && r.json.recommendations && r.json.recommendations.length > 0,
  })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'recommend_slot' });
    return false;
  }
  const locationId = r7a.json.recommendations[0].location.id;

  // 7b. AssignSlot — atomic capacity decrement + assignment + lot advance.
  const r7b = rpc(whTok, '/simaops.warehouse.v1.WarehouseService/AssignSlot', {
    lotId,
    locationId,
    idempotencyKey: `as-${uuid().slice(0, 8)}`,
  }, 'assign_slot');
  if (!check(r7b, { 'assign slot ok': (r) => r.ok })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'assign_slot' });
    return false;
  }

  // 8a. CreateDispatch — lot is now READY_FOR_PRODUCTION; ship it.
  const r8a = rpc(whTok, '/simaops.dispatch.v1.DispatchService/CreateDispatch', {
    lotId,
    destination: 'Load-test DC',
    carrier: 'k6 Logistics',
    quantity: 10,
    unit: 'kg',
    idempotencyKey: `disp-${uuid().slice(0, 8)}`,
  }, 'create_dispatch');
  if (!check(r8a, { 'create dispatch ok': (r) => r.ok && r.json && r.json.dispatch && r.json.dispatch.id })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'create_dispatch' });
    return false;
  }
  const dispatchId = r8a.json.dispatch.id;

  // 8b. UpdateDispatchStatus — advance PENDING → SCHEDULED.
  const r8b = rpc(whTok, '/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus', {
    dispatchId,
    newStatus: 2, // SCHEDULED
    idempotencyKey: `disp-adv-${uuid().slice(0, 8)}`,
  }, 'update_dispatch_status');
  if (!check(r8b, { 'advance dispatch ok': (r) => r.ok })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'update_dispatch_status' });
    return false;
  }

  // 9. GetLotTimeline — audit chain sanity check. Manager role has access.
  const r8 = rpc(admTok, '/simaops.lot.v1.LotService/GetLotTimeline',
    { lotId }, 'get_lot_timeline');
  if (!check(r8, {
    'timeline has entries': (r) => r.ok && r.json && r.json.entries && r.json.entries.length >= 3,
  })) {
    pipelineCompleted.add(1, { result: 'failed', step: 'timeline' });
    return false;
  }

  pipelineDuration.add(Date.now() - t0);
  pipelineCompleted.add(1, { result: 'success' });
  return true;
}

// lib/helpers.js
//
// Test-data generation utilities. Pure JS, no k6-specific imports beyond
// `open()` for fixture loading.

// Load the JPEG fixture once at module init. k6 v2 supports `open()` as a
// synchronous binary read; this happens during the init context so it
// doesn't count toward iteration time.
const QC_IMAGE_BYTES = open('../fixtures/qc-image.jpg', 'b');

// Catalog data — small fixed lists so payloads are recognizable in the DB
// during postmortem. Don't grow these without reason; randomness across
// VUs is enough variety for load.
const SUPPLIERS = [
  'Sumber Tani', 'Cardamom Co', 'Aroma Nusantara', 'Tropical Spice',
  'Sahabat Rempah', 'Indomaharaja', 'Bumi Kencana',
];
const MATERIALS = [
  'Cardamom', 'Sage', 'Lemongrass', 'Cinnamon', 'Clove', 'Nutmeg',
  'Vanilla', 'Pepper', 'Patchouli', 'Citronella',
];
// API enums — see proto/simaops/lot/v1/lot.proto.
const MATERIAL_TYPES   = [1, 2, 3, 4]; // RAW_BOTANICAL / EXTRACT / POWDER / OTHER
const TEMP_RANGES      = [1, 2, 3];    // AMBIENT / COLD / DEEP_FREEZE
// Always use HAZARD_CLASS_NONE for load-test lots. The seed warehouse data
// uses the bare strings ("IBC", "IPPC") in hazard_allowed/drum_compatibility,
// while the API serializes our enum as "HAZARD_CLASS_IBC" — so any non-NONE
// hazard yields 0 RecommendSlot results until that mismatch is fixed
// upstream. The load test deliberately picks NONE to bypass the issue;
// validating the matching logic itself isn't this harness's job.
const HAZARD_CLASSES   = [1];          // NONE only — see comment above

function pick(arr) {
  return arr[Math.floor(Math.random() * arr.length)];
}

// uuidV7 — chronologically sortable, unique enough for idempotency keys
// across thousands of parallel VUs. We don't actually need v7 ordering
// guarantees here; v4 randomness is sufficient. Keeping the v4 path for
// k6's runtime since it lacks crypto.randomUUID.
//
// The output format must satisfy our backend's idempotency-key column
// (VARCHAR(128)). 36-char UUID is well under that.
export function uuid() {
  // RFC 4122 v4 — 16 random bytes, version + variant nibbles set.
  const r = new Uint8Array(16);
  for (let i = 0; i < 16; i++) r[i] = Math.floor(Math.random() * 256);
  r[6] = (r[6] & 0x0f) | 0x40; // version 4
  r[8] = (r[8] & 0x3f) | 0x80; // variant 10
  const hex = Array.from(r, (b) => b.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

// arrival_date — pick a date in the next 0-7 days. The API stores it as a
// DATE column; we send YYYY-MM-DD.
function nearFutureDate() {
  const days = Math.floor(Math.random() * 7);
  const d = new Date(Date.now() + days * 86400 * 1000);
  return d.toISOString().slice(0, 10);
}

// randomLotData — a CreateLotRequest payload with varied content and a
// guaranteed-unique idempotency key tied to the VU + iteration coordinates.
//
// k6 exposes `__VU` (1-based VU id within the scenario) and `__ITER`
// (0-based iteration counter for the current VU). Combined with a per-call
// uuid suffix this is collision-free even under heavy parallelism.
export function randomLotData(__VU, __ITER) {
  return {
    supplierName:  pick(SUPPLIERS),
    materialName:  pick(MATERIALS),
    materialType:  pick(MATERIAL_TYPES),
    quantity:      Math.floor(Math.random() * 90) + 10, // 10..99 kg
    unit:          'kg',
    arrivalDate:   nearFutureDate(),
    storageRequirement: {
      temperatureRange: pick(TEMP_RANGES),
      hazardClass:      pick(HAZARD_CLASSES),
    },
    idempotencyKey: `lot-${__VU}-${__ITER}-${uuid().slice(0, 8)}`,
  };
}

// qcImageBytes — the loaded fixture, as the ArrayBuffer-like object k6
// expects for the `body:` field of an http.put call.
export function qcImageBytes() {
  return QC_IMAGE_BYTES;
}

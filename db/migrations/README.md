# Database Migrations

## JSON Column Shapes

### `lots.storage_requirement`

```json
{
  "temperature_range": "ambient" | "cold" | "deep_freeze",
  "hazard_class": null | "IBC" | "IPPC"
}
```

- `temperature_range` maps to warehouse zone filtering:
  - `ambient` → 15–25 °C
  - `cold` → 2–8 °C
  - `deep_freeze` → −20 to −4 °C
- `hazard_class` filters against `warehouse_locations.hazard_allowed`

### `warehouse_locations.hazard_allowed`

```json
["IBC", "IPPC"]  // or empty []
```

Array of hazard classes this location can store.

### `warehouse_locations.drum_compatibility`

```json
["IBC", "IPPC"]  // or empty []
```

Array of drum types this location physically supports.

### `qc_results.findings_json`

```json
[
  {
    "class_name": "bottle",
    "mapped_finding": "foreign_matter",
    "confidence": 0.87,
    "is_anomaly": true
  }
]
```

Array of detected findings from the AI model, mapped via `findings_map.yaml`.

### `outbox_events.payload_json`

```json
{
  "qc_job_id": "uuid",
  "lot_id": "uuid",
  "material_type": "RAW_BOTANICAL",
  "image_object_key": "simaops-qc-images/lot-id/file.jpg"
}
```

Shape varies by `event_type`. The consumer (AI worker) uses `event_type` to determine parsing.

### `audit_logs.before_json` / `after_json`

Full entity snapshot as JSON. `before_json` is null on create; `after_json` is null on delete.

## Running Migrations

```bash
# Against local TiDB (docker compose)
make db-migrate

# Manual
mysql -h 127.0.0.1 -P 4000 -u root < db/migrations/001_initial_schema.sql
```

# Audit Log

## Entry Shape

```json
{
  "id": "uuid",
  "actor_user_id": "keycloak-sub-uuid",
  "actor_role": "OPERATOR",
  "action": "lot.create",
  "entity_type": "lot",
  "entity_id": "lot-uuid",
  "before_json": null,
  "after_json": "{...lot snapshot...}",
  "request_id": "uuid",
  "trace_id": "otel-trace-id",
  "created_at": "2026-05-26T10:00:00Z"
}
```

## Actions Logged

- `lot.create`, `lot.status_change`
- `qc.job.create`, `qc.review`
- `warehouse.assign`
- `dispatch.created`, `dispatch.status_changed`
- `admin.role.assign`, `admin.role.revoke`

## Retention

- No automatic deletion in v1
- Indexed on `(entity_type, entity_id)` for timeline queries
- Indexed on `actor_user_id` for per-user audit
- Indexed on `created_at` for chronological listing

## Access

- `AuditService.ListAuditLogs` — paginated, MANAGER + ADMIN only
- `AuditService.GetEntityAuditTrail` — per-entity chronological, MANAGER + ADMIN only
- Lot detail page "Timeline" tab — shows entity trail inline

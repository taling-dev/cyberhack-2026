package middleware

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
)

// rpcAuditAction maps Connect RPC paths to (action_name, entity_type, body_field_for_entity_id).
// Only mutation RPCs are audited.
var rpcAuditAction = map[string]struct {
	Action     string
	EntityType string
	IDField    string // JSON field in request body that holds the entity ID
}{
	"/simaops.lot.v1.LotService/CreateLot":                    {"lot.created", "lot", ""},
	"/simaops.lot.v1.LotService/UpdateLotStatus":              {"lot.status_changed", "lot", "lotId"},
	"/simaops.qc.v1.QCService/CreateQCJob":                    {"qc.job_created", "qc_job", ""}, // use response.job.id
	"/simaops.qc.v1.QCService/CreateQCUploadUrl":              {"qc.upload_requested", "lot", "lotId"},
	"/simaops.qc.v1.QCService/ReviewQC":                       {"qc.reviewed", "qc_job", "qcJobId"},
	"/simaops.qc.v1.QCService/RetryQCJob":                     {"qc.retry", "qc_job", "qcJobId"},
	"/simaops.warehouse.v1.WarehouseService/AssignSlot":       {"warehouse.assigned", "lot", "lotId"},
	"/simaops.dispatch.v1.DispatchService/CreateDispatch":       {"dispatch.created", "dispatch", ""}, // use response.dispatch.id
	"/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus": {"dispatch.status_changed", "dispatch", "dispatchId"},
	"/simaops.admin.v1.AdminService/AssignRole":               {"admin.role_assigned", "user", "userId"},
	"/simaops.admin.v1.AdminService/RevokeRole":               {"admin.role_revoked", "user", "userId"},
}

// Audit middleware records mutation RPCs to the audit_logs table.
// Runs AFTER the handler so it can inspect both request and response.
func Audit(dbConn *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		auditCfg, isAudited := rpcAuditAction[path]
		if !isAudited {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer request body so we can read it AND pass it to the handler
		body, err := io.ReadAll(r.Body)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		// Capture response
		rec := &auditResponseRecorder{ResponseWriter: w, body: &bytes.Buffer{}, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		// Only audit successful mutations (2xx)
		if rec.status < 200 || rec.status >= 300 {
			return
		}

		// Extract user from claims
		actorUserID := "anonymous"
		actorRole := "system"
		if claims := auth.GetClaims(r.Context()); claims != nil {
			if claims.Username != "" {
				actorUserID = claims.Username
			} else if claims.Sub != "" {
				actorUserID = claims.Sub
			}
			if len(claims.Roles) > 0 {
				actorRole = primaryRole(claims.Roles)
			}
		}

		// Determine entity ID
		entityID := extractEntityID(body, auditCfg.IDField, rec.body.Bytes(), auditCfg.EntityType)

		// Request ID from middleware
		requestID := r.Header.Get("X-Request-Id")
		if requestID == "" {
			if v, ok := r.Context().Value(RequestIDKey).(string); ok {
				requestID = v
			}
		}

		// Write audit log with bounded timeout, propagating trace context.
		// Detached from the request context (which is being completed) but with a
		// 5s deadline to prevent hung goroutines.
		traceCtx := trace.ContextWithSpanContext(context.Background(), trace.SpanContextFromContext(r.Context()))
		writeCtx, cancel := context.WithTimeout(traceCtx, 5*time.Second)
		go func() {
			defer cancel()
			if err := createAuditLog(writeCtx, dbConn, auditLogEntry{
				ID:          uuid.NewString(),
				ActorUserID: actorUserID,
				ActorRole:   actorRole,
				Action:      auditCfg.Action,
				EntityType:  auditCfg.EntityType,
				EntityID:    entityID,
				BeforeJSON:  nil,
				AfterJSON:   body,
				RequestID:   requestID,
			}); err != nil {
				// Don't fail the request — log and continue. Operators can detect
				// audit gaps via the simaops_audit_failures_total metric.
				slog.Default().Warn("audit log write failed",
					"action", auditCfg.Action,
					"err", err,
					"trace_id", trace.SpanFromContext(traceCtx).SpanContext().TraceID().String(),
				)
			}
		}()
	})
}

type auditLogEntry struct {
	ID          string
	ActorUserID string
	ActorRole   string
	Action      string
	EntityType  string
	EntityID    string
	BeforeJSON  []byte
	AfterJSON   []byte
	RequestID   string
}

func createAuditLog(ctx context.Context, db *sql.DB, e auditLogEntry) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO audit_logs (id, actor_user_id, actor_role, action, entity_type, entity_id, before_json, after_json, request_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ActorUserID, e.ActorRole, e.Action, e.EntityType, e.EntityID,
		nullableJSON(e.BeforeJSON), nullableJSON(e.AfterJSON), nullString(e.RequestID),
	)
	return err
}

func nullableJSON(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	return b
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// extractEntityID reads either the request body field (e.g. "lotId") or, for
// CreateLot, looks at the response to get the new lot's ID.
func extractEntityID(reqBody []byte, idField string, respBody []byte, entityType string) string {
	if idField != "" {
		var m map[string]interface{}
		if json.Unmarshal(reqBody, &m) == nil {
			if v, ok := m[idField].(string); ok {
				return v
			}
		}
	}

	// CreateLot — response is { "lot": { "id": "..." } }
	if entityType == "lot" {
		var resp struct {
			Lot struct {
				Id string `json:"id"`
			} `json:"lot"`
		}
		if json.Unmarshal(respBody, &resp) == nil && resp.Lot.Id != "" {
			return resp.Lot.Id
		}
	}

	// CreateQCJob — response is { "job": { "id": "..." } }
	if entityType == "qc_job" {
		var resp struct {
			Job struct {
				Id string `json:"id"`
			} `json:"job"`
		}
		if json.Unmarshal(respBody, &resp) == nil && resp.Job.Id != "" {
			return resp.Job.Id
		}
	}

	// CreateDispatch — response is { "dispatch": { "id": "..." } }
	if entityType == "dispatch" {
		var resp struct {
			Dispatch struct {
				Id string `json:"id"`
			} `json:"dispatch"`
		}
		if json.Unmarshal(respBody, &resp) == nil && resp.Dispatch.Id != "" {
			return resp.Dispatch.Id
		}
	}

	return ""
}

func primaryRole(roles []string) string {
	priority := []string{"ADMIN", "MANAGER", "QC_SUPERVISOR", "WAREHOUSE_STAFF", "OPERATOR"}
	for _, p := range priority {
		for _, r := range roles {
			if r == p {
				return r
			}
		}
	}
	if len(roles) > 0 {
		return roles[0]
	}
	return "user"
}

type auditResponseRecorder struct {
	http.ResponseWriter
	body   *bytes.Buffer
	status int
}

func (r *auditResponseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *auditResponseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying writer for http.NewResponseController so SSE
// streams traversing this middleware can still flush.
func (r *auditResponseRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

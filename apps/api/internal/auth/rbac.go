package auth

import (
	"net/http"
	"strings"
)

// rpcRoles maps Connect RPC procedure paths to the roles allowed to call them.
// Empty slice = public (any authenticated user). Nil = no restriction (health endpoints).
var rpcRoles = map[string][]string{
	// LotService — OPERATOR, ADMIN can create; all authenticated can read
	"/simaops.lot.v1.LotService/CreateLot":       {"OPERATOR", "ADMIN"},
	"/simaops.lot.v1.LotService/GetLot":          {},
	"/simaops.lot.v1.LotService/ListLots":        {},
	"/simaops.lot.v1.LotService/UpdateLotStatus": {"OPERATOR", "ADMIN"},
	"/simaops.lot.v1.LotService/GetLotTimeline":  {},

	// QCService — OPERATOR uploads/creates; QC_SUPERVISOR reviews
	"/simaops.qc.v1.QCService/CreateQCUploadUrl": {"OPERATOR", "ADMIN"},
	"/simaops.qc.v1.QCService/CreateQCViewUrl":   {},
	"/simaops.qc.v1.QCService/CreateQCJob":       {"OPERATOR", "ADMIN"},
	"/simaops.qc.v1.QCService/GetQCJob":          {},
	"/simaops.qc.v1.QCService/GetQCResult":       {},
	"/simaops.qc.v1.QCService/ReviewQC":          {"QC_SUPERVISOR", "ADMIN"},
	"/simaops.qc.v1.QCService/RetryQCJob":        {"QC_SUPERVISOR", "ADMIN"},

	// WarehouseService — WAREHOUSE_STAFF assigns
	"/simaops.warehouse.v1.WarehouseService/ListLocations":          {},
	"/simaops.warehouse.v1.WarehouseService/RecommendSlot":          {"WAREHOUSE_STAFF", "ADMIN"},
	"/simaops.warehouse.v1.WarehouseService/AssignSlot":             {"WAREHOUSE_STAFF", "ADMIN"},
	"/simaops.warehouse.v1.WarehouseService/GetWarehouseAssignments": {},

	// AuditService — MANAGER, ADMIN
	"/simaops.audit.v1.AuditService/ListAuditLogs":       {"MANAGER", "ADMIN"},
	"/simaops.audit.v1.AuditService/GetEntityAuditTrail": {"MANAGER", "ADMIN"},

	// DashboardService — read-only aggregate operational metrics. The
	// dashboard is the universal post-login landing page for every role
	// (see web nav: /dashboard is visible to all roles), and these RPCs
	// return only non-sensitive summary counts, so any authenticated user
	// may read them — same pattern as GetLot/ListLots/ListLocations.
	"/simaops.dashboard.v1.DashboardService/GetOpsDashboard":    {},
	"/simaops.dashboard.v1.DashboardService/GetQCMetrics":       {},
	"/simaops.dashboard.v1.DashboardService/GetWarehouseMetrics": {},

	// AdminService — ADMIN only
	"/simaops.admin.v1.AdminService/ListUsers":  {"ADMIN"},
	"/simaops.admin.v1.AdminService/AssignRole": {"ADMIN"},
	"/simaops.admin.v1.AdminService/RevokeRole": {"ADMIN"},
	"/simaops.admin.v1.AdminService/ListRoles":  {"ADMIN"},
}

// RBACMiddleware enforces role-based access on Connect RPC procedures.
func RBACMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Skip non-RPC paths (health, etc.)
		if !strings.Contains(path, "/simaops.") {
			next.ServeHTTP(w, r)
			return
		}

		requiredRoles, exists := rpcRoles[path]
		if !exists {
			// Unknown RPC — deny by default
			http.Error(w, `{"code":"permission_denied","message":"unknown procedure"}`, http.StatusForbidden)
			return
		}

		// Empty slice = any authenticated user
		if len(requiredRoles) == 0 {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"code":"unauthenticated","message":"authentication required"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		// Check role membership
		claims := GetClaims(r.Context())
		if claims == nil {
			http.Error(w, `{"code":"unauthenticated","message":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		if !hasAnyRole(claims.Roles, requiredRoles) {
			http.Error(w, `{"code":"permission_denied","message":"insufficient role"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasAnyRole(userRoles, required []string) bool {
	for _, req := range required {
		for _, ur := range userRoles {
			if ur == req {
				return true
			}
		}
	}
	return false
}

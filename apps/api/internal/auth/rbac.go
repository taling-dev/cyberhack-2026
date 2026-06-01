package auth

import (
	"net/http"
	"strings"
	"sync"
)

// publicProcedures are callable by any authenticated user (read-only or
// universally-needed RPCs). These stay in code: they are not a per-role grant.
var publicProcedures = map[string]bool{
	"/simaops.lot.v1.LotService/GetLot":                            true,
	"/simaops.lot.v1.LotService/ListLots":                          true,
	"/simaops.lot.v1.LotService/GetLotTimeline":                    true,
	"/simaops.qc.v1.QCService/CreateQCViewUrl":                     true,
	"/simaops.qc.v1.QCService/GetQCJob":                            true,
	"/simaops.qc.v1.QCService/GetQCResult":                         true,
	"/simaops.warehouse.v1.WarehouseService/ListLocations":         true,
	"/simaops.warehouse.v1.WarehouseService/GetWarehouseAssignments": true,
	"/simaops.dispatch.v1.DispatchService/GetDispatch":             true,
	"/simaops.dispatch.v1.DispatchService/ListDispatches":          true,
	"/simaops.dashboard.v1.DashboardService/GetOpsDashboard":       true,
	"/simaops.dashboard.v1.DashboardService/GetQCMetrics":          true,
	"/simaops.dashboard.v1.DashboardService/GetWarehouseMetrics":   true,
	"/simaops.dashboard.v1.DashboardService/GetQCTrend":            true,
	"/simaops.dashboard.v1.DashboardService/GetLatestInspection":   true,
}

// adminOnlyProcedures require ADMIN specifically (admin console). Kept in code
// so a custom role can never be granted user/role administration.
var adminOnlyProcedures = map[string]bool{
	"/simaops.admin.v1.AdminService/ListUsers":      true,
	"/simaops.admin.v1.AdminService/AssignRole":     true,
	"/simaops.admin.v1.AdminService/RevokeRole":     true,
	"/simaops.admin.v1.AdminService/ListRoles":      true,
	"/simaops.admin.v1.AdminService/CreateRole":     true,
	"/simaops.admin.v1.AdminService/DeleteRole":     true,
	"/simaops.admin.v1.AdminService/ListProcedures": true,
	"/simaops.admin.v1.AdminService/CreateUser":     true,
	"/simaops.admin.v1.AdminService/UpdateUser":     true,
	"/simaops.admin.v1.AdminService/UpdateRole":     true,
}

// AllGrantableProcedures is the set of RPCs a custom role can be granted (the
// non-public, non-admin procedures). Exposed via AdminService.ListProcedures
// so the admin UI offers a valid checklist. Order-stable for display.
var AllGrantableProcedures = []string{
	"/simaops.lot.v1.LotService/CreateLot",
	"/simaops.lot.v1.LotService/UpdateLotStatus",
	"/simaops.qc.v1.QCService/CreateQCUploadUrl",
	"/simaops.qc.v1.QCService/CreateQCJob",
	"/simaops.qc.v1.QCService/ReviewQC",
	"/simaops.qc.v1.QCService/RetryQCJob",
	"/simaops.warehouse.v1.WarehouseService/RecommendSlot",
	"/simaops.warehouse.v1.WarehouseService/AssignSlot",
	"/simaops.dispatch.v1.DispatchService/CreateDispatch",
	"/simaops.dispatch.v1.DispatchService/UpdateDispatchStatus",
	"/simaops.audit.v1.AuditService/ListAuditLogs",
	"/simaops.audit.v1.AuditService/GetEntityAuditTrail",
}

// permStore holds the data-driven role->procedures grants, refreshable at
// runtime when an admin mutates roles. Reads are lock-free-ish via RWMutex.
type permStore struct {
	mu    sync.RWMutex
	grant map[string]map[string]bool // rpc_path -> set of role names allowed
}

var perms = &permStore{grant: map[string]map[string]bool{}}

// SetRolePermissions replaces the in-memory grant table. Pairs is a flat list
// of (roleName, rpcPath). Called at startup and after any role mutation.
func SetRolePermissions(pairs [][2]string) {
	g := make(map[string]map[string]bool)
	for _, p := range pairs {
		roleName, rpc := p[0], p[1]
		if g[rpc] == nil {
			g[rpc] = map[string]bool{}
		}
		g[rpc][roleName] = true
	}
	perms.mu.Lock()
	perms.grant = g
	perms.mu.Unlock()
}

func (s *permStore) allows(rpc string, userRoles []string) bool {
	s.mu.RLock()
	allowed := s.grant[rpc]
	s.mu.RUnlock()
	for _, ur := range userRoles {
		if allowed[ur] {
			return true
		}
	}
	return false
}

func isKnownProcedure(path string) bool {
	if publicProcedures[path] || adminOnlyProcedures[path] {
		return true
	}
	for _, p := range AllGrantableProcedures {
		if p == path {
			return true
		}
	}
	return false
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

		if !isKnownProcedure(path) {
			http.Error(w, `{"code":"permission_denied","message":"unknown procedure"}`, http.StatusForbidden)
			return
		}

		claims := GetClaims(r.Context())
		if claims == nil {
			http.Error(w, `{"code":"unauthenticated","message":"authentication required"}`, http.StatusUnauthorized)
			return
		}

		// ADMIN bypasses all non-public checks.
		for _, role := range claims.Roles {
			if role == "ADMIN" {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Public procedures: any authenticated user.
		if publicProcedures[path] {
			next.ServeHTTP(w, r)
			return
		}

		// Admin-only procedures: only ADMIN (handled above) may pass.
		if adminOnlyProcedures[path] {
			http.Error(w, `{"code":"permission_denied","message":"insufficient role"}`, http.StatusForbidden)
			return
		}

		// Data-driven per-role grants for everything else.
		if !perms.allows(path, claims.Roles) {
			http.Error(w, `{"code":"permission_denied","message":"insufficient role"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

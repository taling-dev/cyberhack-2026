package handler

import (
	"database/sql"
	"net/http"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1/adminv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/audit/v1/auditv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1/dashboardv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dispatch/v1/dispatchv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1/lotv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1/warehousev1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

// RegisterConnectHandlers mounts Connect RPC service handlers on the mux.
// hub is optional — pass nil if SSE/event integration is not wired (eg in tests).
func RegisterConnectHandlers(mux *http.ServeMux, dbConn *sql.DB, minio *storage.MinIOClient, hub *events.Hub) {
	queries := db.New(dbConn)

	// LotService
	lotPath, lotHandler := lotv1connect.NewLotServiceHandler(NewLotService(queries, dbConn))
	mux.Handle(lotPath, lotHandler)

	// WarehouseService — shared with QCService so approval can auto-assign.
	whSvc := NewWarehouseService(queries, dbConn)

	// QCService
	qcPath, qcHandler := qcv1connect.NewQCServiceHandler(NewQCService(queries, dbConn, minio, whSvc))
	mux.Handle(qcPath, qcHandler)

	// WarehouseService
	whPath, whHandler := warehousev1connect.NewWarehouseServiceHandler(whSvc)
	mux.Handle(whPath, whHandler)

	// DispatchService — final stage: ships READY_FOR_PRODUCTION lots.
	dispatchPath, dispatchHandler := dispatchv1connect.NewDispatchServiceHandler(NewDispatchService(queries, dbConn))
	mux.Handle(dispatchPath, dispatchHandler)

	// AuditService
	auditPath, auditHandler := auditv1connect.NewAuditServiceHandler(NewAuditService(queries))
	mux.Handle(auditPath, auditHandler)

	// DashboardService
	dashPath, dashHandler := dashboardv1connect.NewDashboardServiceHandler(NewDashboardService(queries))
	mux.Handle(dashPath, dashHandler)

	// AdminService — receives the hub so AssignRole / RevokeRole can kick the
	// affected user's open SSE connections, forcing reconnect with the new
	// role list.
	adminPath, adminHandler := adminv1connect.NewAdminServiceHandler(NewAdminService(queries, hub))
	mux.Handle(adminPath, adminHandler)
}

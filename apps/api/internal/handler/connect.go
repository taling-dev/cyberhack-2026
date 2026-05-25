package handler

import (
	"database/sql"
	"net/http"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1/adminv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/audit/v1/auditv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/dashboard/v1/dashboardv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1/lotv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/qc/v1/qcv1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/warehouse/v1/warehousev1connect"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/storage"
)

// RegisterConnectHandlers mounts Connect RPC service handlers on the mux.
func RegisterConnectHandlers(mux *http.ServeMux, dbConn *sql.DB, minio *storage.MinIOClient) {
	queries := db.New(dbConn)

	// LotService
	lotPath, lotHandler := lotv1connect.NewLotServiceHandler(NewLotService(queries))
	mux.Handle(lotPath, lotHandler)

	// QCService
	qcPath, qcHandler := qcv1connect.NewQCServiceHandler(NewQCService(queries, minio))
	mux.Handle(qcPath, qcHandler)

	// WarehouseService
	whPath, whHandler := warehousev1connect.NewWarehouseServiceHandler(NewWarehouseService(queries))
	mux.Handle(whPath, whHandler)

	// AuditService
	auditPath, auditHandler := auditv1connect.NewAuditServiceHandler(NewAuditService(queries))
	mux.Handle(auditPath, auditHandler)

	// DashboardService
	dashPath, dashHandler := dashboardv1connect.NewDashboardServiceHandler(NewDashboardService(queries))
	mux.Handle(dashPath, dashHandler)

	// AdminService
	adminPath, adminHandler := adminv1connect.NewAdminServiceHandler(NewAdminService(queries))
	mux.Handle(adminPath, adminHandler)
}

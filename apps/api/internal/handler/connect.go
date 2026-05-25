package handler

import (
	"database/sql"
	"net/http"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
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

	// Remaining services — stubs
	mux.Handle("/simaops.audit.v1.AuditService/", newUnimplementedHandler())
	mux.Handle("/simaops.dashboard.v1.DashboardService/", newUnimplementedHandler())
	mux.Handle("/simaops.admin.v1.AdminService/", newUnimplementedHandler())
}

func newUnimplementedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := connect.NewError(connect.CodeUnimplemented, nil)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"code":"` + err.Code().String() + `"}`))
	})
}

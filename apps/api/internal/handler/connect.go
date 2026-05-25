package handler

import (
	"database/sql"
	"net/http"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/lot/v1/lotv1connect"
)

// RegisterConnectHandlers mounts Connect RPC service handlers on the mux.
func RegisterConnectHandlers(mux *http.ServeMux, dbConn *sql.DB) {
	queries := db.New(dbConn)

	// LotService — real implementation
	lotPath, lotHandler := lotv1connect.NewLotServiceHandler(NewLotService(queries))
	mux.Handle(lotPath, lotHandler)

	// Remaining services — stubs until implemented
	mux.Handle("/simaops.qc.v1.QCService/", newUnimplementedHandler())
	mux.Handle("/simaops.warehouse.v1.WarehouseService/", newUnimplementedHandler())
	mux.Handle("/simaops.audit.v1.AuditService/", newUnimplementedHandler())
	mux.Handle("/simaops.dashboard.v1.DashboardService/", newUnimplementedHandler())
	mux.Handle("/simaops.admin.v1.AdminService/", newUnimplementedHandler())
}

func newUnimplementedHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := connect.NewError(connect.CodeUnimplemented, nil)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		writeConnectError(w, err)
	})
}

func writeConnectError(w http.ResponseWriter, err *connect.Error) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"code":"` + err.Code().String() + `"}`))
}

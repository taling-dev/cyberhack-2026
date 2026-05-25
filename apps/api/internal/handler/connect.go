package handler

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
)

// RegisterConnectHandlers mounts Connect RPC service handlers on the mux.
// Currently registers a no-op LotService that returns Unimplemented for all methods.
func RegisterConnectHandlers(mux *http.ServeMux) {
	// LotService stub — will be replaced with generated handler in Task 7
	mux.Handle("/simaops.lot.v1.LotService/", newUnimplementedHandler())
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
		// Connect protocol error response
		_ = writeConnectError(w, err)
	})
}

func writeConnectError(w http.ResponseWriter, err *connect.Error) error {
	w.Header().Set("Content-Type", "application/json")
	_, e := w.Write([]byte(`{"code":"` + err.Code().String() + `"}`))
	return e
}

// Ensure connect package is used (compile check).
var _ context.Context

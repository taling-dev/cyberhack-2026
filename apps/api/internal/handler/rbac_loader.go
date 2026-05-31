package handler

import (
	"context"
	"log/slog"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
)

// refreshRolePermissions reloads the data-driven RBAC grant table from the DB
// into the auth permStore. Called at startup and after any role mutation so a
// newly created/changed role takes effect without a restart.
func refreshRolePermissions(ctx context.Context, q *db.Queries) {
	rows, err := q.ListAllRolePermissions(ctx)
	if err != nil {
		slog.Error("refresh role permissions failed", "err", err)
		return
	}
	pairs := make([][2]string, 0, len(rows))
	for _, r := range rows {
		pairs = append(pairs, [2]string{r.RoleName, r.RpcPath})
	}
	auth.SetRolePermissions(pairs)
}

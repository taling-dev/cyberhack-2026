package handler

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	adminv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1/adminv1connect"
)

var _ adminv1connect.AdminServiceHandler = (*AdminService)(nil)

type AdminService struct {
	q *db.Queries
}

func NewAdminService(q *db.Queries) *AdminService {
	return &AdminService{q: q}
}

func (s *AdminService) ListUsers(ctx context.Context, req *connect.Request[adminv1.ListUsersRequest]) (*connect.Response[adminv1.ListUsersResponse], error) {
	pageSize := int32(50)
	if req.Msg.PageSize > 0 {
		pageSize = req.Msg.PageSize
	}

	users, err := s.q.ListUsers(ctx, db.ListUsersParams{Limit: pageSize, Offset: 0})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list users: %w", err))
	}

	totalCount, _ := s.q.CountUsers(ctx)

	protoUsers := make([]*adminv1.User, 0, len(users))
	for _, u := range users {
		var roles []adminv1.Role
		// role_names is GROUP_CONCAT of comma-separated names
		if u.RoleNames.Valid && u.RoleNames.String != "" {
			for _, name := range strings.Split(u.RoleNames.String, ",") {
				roles = append(roles, roleNameToProto(strings.TrimSpace(name)))
			}
		}
		protoUsers = append(protoUsers, &adminv1.User{
			Id:       u.ID,
			Username: u.Username,
			Email:    u.Email,
			FullName: u.FullName,
			Roles:    roles,
			Active:   u.Active,
		})
	}

	return connect.NewResponse(&adminv1.ListUsersResponse{
		Users:      protoUsers,
		TotalCount: int32(totalCount),
	}), nil
}

func (s *AdminService) AssignRole(ctx context.Context, req *connect.Request[adminv1.AssignRoleRequest]) (*connect.Response[adminv1.AssignRoleResponse], error) {
	if req.Msg.UserId == "" || req.Msg.Role == adminv1.Role_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	// Note: full implementation requires Keycloak Admin API sync
	return connect.NewResponse(&adminv1.AssignRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId, Roles: []adminv1.Role{req.Msg.Role}},
	}), nil
}

func (s *AdminService) RevokeRole(ctx context.Context, req *connect.Request[adminv1.RevokeRoleRequest]) (*connect.Response[adminv1.RevokeRoleResponse], error) {
	if req.Msg.UserId == "" || req.Msg.Role == adminv1.Role_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	return connect.NewResponse(&adminv1.RevokeRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId},
	}), nil
}

func (s *AdminService) ListRoles(ctx context.Context, req *connect.Request[adminv1.ListRolesRequest]) (*connect.Response[adminv1.ListRolesResponse], error) {
	roles, err := s.q.ListRoles(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list roles: %w", err))
	}

	descriptions := map[string]string{
		"OPERATOR":         "Lot intake and QC image upload",
		"QC_SUPERVISOR":    "Approve/reject QC results",
		"WAREHOUSE_STAFF":  "Assign warehouse slots",
		"MANAGER":          "View dashboards and audit logs",
		"ADMIN":            "Full system access",
	}

	protoRoles := make([]*adminv1.RoleDefinition, 0, len(roles))
	for _, r := range roles {
		protoRoles = append(protoRoles, &adminv1.RoleDefinition{
			Role:        roleNameToProto(r.Name),
			Name:        r.Name,
			Description: descriptions[r.Name],
		})
	}

	return connect.NewResponse(&adminv1.ListRolesResponse{Roles: protoRoles}), nil
}

func roleNameToProto(name string) adminv1.Role {
	switch name {
	case "OPERATOR":
		return adminv1.Role_ROLE_OPERATOR
	case "QC_SUPERVISOR":
		return adminv1.Role_ROLE_QC_SUPERVISOR
	case "WAREHOUSE_STAFF":
		return adminv1.Role_ROLE_WAREHOUSE_STAFF
	case "MANAGER":
		return adminv1.Role_ROLE_MANAGER
	case "ADMIN":
		return adminv1.Role_ROLE_ADMIN
	default:
		return adminv1.Role_ROLE_UNSPECIFIED
	}
}

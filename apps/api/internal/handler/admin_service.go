package handler

import (
	"context"
	"fmt"

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
	// For now, return users from users_profile table
	// TODO: join with user_roles for role info
	return connect.NewResponse(&adminv1.ListUsersResponse{
		Users: []*adminv1.User{
			{Id: "u-operator", Username: "operator", Email: "operator@simaops.local", FullName: "Budi Operator", Roles: []adminv1.Role{adminv1.Role_ROLE_OPERATOR}, Active: true},
			{Id: "u-qc-supervisor", Username: "qc_supervisor", Email: "qc@simaops.local", FullName: "Siti QC Supervisor", Roles: []adminv1.Role{adminv1.Role_ROLE_QC_SUPERVISOR}, Active: true},
			{Id: "u-warehouse", Username: "warehouse", Email: "warehouse@simaops.local", FullName: "Agus Warehouse", Roles: []adminv1.Role{adminv1.Role_ROLE_WAREHOUSE_STAFF}, Active: true},
			{Id: "u-manager", Username: "manager", Email: "manager@simaops.local", FullName: "Dewi Manager", Roles: []adminv1.Role{adminv1.Role_ROLE_MANAGER}, Active: true},
			{Id: "u-admin", Username: "admin", Email: "admin@simaops.local", FullName: "Andi Admin", Roles: []adminv1.Role{adminv1.Role_ROLE_ADMIN}, Active: true},
		},
		TotalCount: 5,
	}), nil
}

func (s *AdminService) AssignRole(ctx context.Context, req *connect.Request[adminv1.AssignRoleRequest]) (*connect.Response[adminv1.AssignRoleResponse], error) {
	if req.Msg.UserId == "" || req.Msg.Role == adminv1.Role_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	// TODO: write to user_roles table + sync to Keycloak Admin API
	return connect.NewResponse(&adminv1.AssignRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId, Roles: []adminv1.Role{req.Msg.Role}},
	}), nil
}

func (s *AdminService) RevokeRole(ctx context.Context, req *connect.Request[adminv1.RevokeRoleRequest]) (*connect.Response[adminv1.RevokeRoleResponse], error) {
	if req.Msg.UserId == "" || req.Msg.Role == adminv1.Role_ROLE_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	// TODO: delete from user_roles table + sync to Keycloak Admin API
	return connect.NewResponse(&adminv1.RevokeRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId},
	}), nil
}

func (s *AdminService) ListRoles(ctx context.Context, req *connect.Request[adminv1.ListRolesRequest]) (*connect.Response[adminv1.ListRolesResponse], error) {
	return connect.NewResponse(&adminv1.ListRolesResponse{
		Roles: []*adminv1.RoleDefinition{
			{Role: adminv1.Role_ROLE_OPERATOR, Name: "OPERATOR", Description: "Lot intake and QC image upload"},
			{Role: adminv1.Role_ROLE_QC_SUPERVISOR, Name: "QC_SUPERVISOR", Description: "Approve/reject QC results"},
			{Role: adminv1.Role_ROLE_WAREHOUSE_STAFF, Name: "WAREHOUSE_STAFF", Description: "Assign warehouse slots"},
			{Role: adminv1.Role_ROLE_MANAGER, Name: "MANAGER", Description: "View dashboards and audit logs"},
			{Role: adminv1.Role_ROLE_ADMIN, Name: "ADMIN", Description: "Full system access"},
		},
	}), nil
}

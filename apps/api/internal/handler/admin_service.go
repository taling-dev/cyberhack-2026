package handler

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/events"
	adminv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1/adminv1connect"
)

var _ adminv1connect.AdminServiceHandler = (*AdminService)(nil)

type AdminService struct {
	q   *db.Queries
	hub *events.Hub // optional — if non-nil, role mutations auto-kick the affected user
	kc  *auth.KeycloakAdmin // mirrors role changes to Keycloak (no-op if unconfigured)
}

func NewAdminService(q *db.Queries, hub *events.Hub) *AdminService {
	return &AdminService{q: q, hub: hub, kc: auth.NewKeycloakAdmin()}
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
		var roleNames []string
		// role_names is GROUP_CONCAT of comma-separated names
		if u.RoleNames.Valid && u.RoleNames.String != "" {
			for _, name := range strings.Split(u.RoleNames.String, ",") {
				n := strings.TrimSpace(name)
				roleNames = append(roleNames, n)
				roles = append(roles, roleNameToProto(n))
			}
		}
		protoUsers = append(protoUsers, &adminv1.User{
			Id:        u.ID,
			Username:  u.Username,
			Email:     u.Email,
			FullName:  u.FullName,
			Roles:     roles,
			RoleNames: roleNames,
			Active:    u.Active,
		})
	}

	return connect.NewResponse(&adminv1.ListUsersResponse{
		Users:      protoUsers,
		TotalCount: int32(totalCount),
	}), nil
}

func (s *AdminService) AssignRole(ctx context.Context, req *connect.Request[adminv1.AssignRoleRequest]) (*connect.Response[adminv1.AssignRoleResponse], error) {
	roleName := req.Msg.RoleName
	if roleName == "" {
		roleName = protoToRoleName(req.Msg.Role)
	}
	if req.Msg.UserId == "" || roleName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	role, err := s.q.GetRoleByName(ctx, roleName)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role %s not found", roleName))
	}
	if err := s.q.AssignUserRole(ctx, db.AssignUserRoleParams{
		UserID: req.Msg.UserId,
		RoleID: role.ID,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("assign role: %w", err))
	}
	// Mirror to Keycloak so the user's JWT carries the new role on next refresh.
	// Resolve the Keycloak username from the local user record (local ids are
	// synthetic placeholders, not the KC sub). No-op when the admin service
	// account isn't configured.
	if u, uErr := s.q.GetUserByID(ctx, req.Msg.UserId); uErr == nil {
		if err := s.kc.AssignRealmRole(ctx, u.Username, roleName); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("keycloak role sync: %w", err))
		}
	}
	// Kick the user's open SSE connections so the next reconnect carries the
	// new role list.
	if s.hub != nil {
		s.hub.KickUser(req.Msg.UserId)
	}
	roleNames, _ := s.q.ListUserRoleNames(ctx, req.Msg.UserId)
	roles := make([]adminv1.Role, 0, len(roleNames))
	for _, rn := range roleNames {
		roles = append(roles, roleNameToProto(rn))
	}
	return connect.NewResponse(&adminv1.AssignRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId, Roles: roles, RoleNames: roleNames},
	}), nil
}

func (s *AdminService) RevokeRole(ctx context.Context, req *connect.Request[adminv1.RevokeRoleRequest]) (*connect.Response[adminv1.RevokeRoleResponse], error) {
	roleName := req.Msg.RoleName
	if roleName == "" {
		roleName = protoToRoleName(req.Msg.Role)
	}
	if req.Msg.UserId == "" || roleName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("user_id and role required"))
	}
	role, err := s.q.GetRoleByName(ctx, roleName)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role %s not found", roleName))
	}
	if err := s.q.RevokeUserRole(ctx, db.RevokeUserRoleParams{
		UserID: req.Msg.UserId,
		RoleID: role.ID,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("revoke role: %w", err))
	}
	if u, uErr := s.q.GetUserByID(ctx, req.Msg.UserId); uErr == nil {
		if err := s.kc.RemoveRealmRole(ctx, u.Username, roleName); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("keycloak role sync: %w", err))
		}
	}
	if s.hub != nil {
		s.hub.KickUser(req.Msg.UserId)
	}
	roleNames, _ := s.q.ListUserRoleNames(ctx, req.Msg.UserId)
	roles := make([]adminv1.Role, 0, len(roleNames))
	for _, rn := range roleNames {
		roles = append(roles, roleNameToProto(rn))
	}
	return connect.NewResponse(&adminv1.RevokeRoleResponse{
		User: &adminv1.User{Id: req.Msg.UserId, Roles: roles, RoleNames: roleNames},
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

	// Per-role granted RPC paths, from the data-driven grant table.
	permRows, _ := s.q.ListAllRolePermissions(ctx)
	permsByRole := map[string][]string{}
	for _, pr := range permRows {
		permsByRole[pr.RoleName] = append(permsByRole[pr.RoleName], pr.RpcPath)
	}

	protoRoles := make([]*adminv1.RoleDefinition, 0, len(roles))
	for _, r := range roles {
		desc := r.Description
		if desc == "" {
			desc = descriptions[r.Name]
		}
		protoRoles = append(protoRoles, &adminv1.RoleDefinition{
			Role:        roleNameToProto(r.Name),
			Name:        r.Name,
			Description: desc,
			IsSystem:    r.IsSystem,
			Permissions: permsByRole[r.Name],
			Id:          r.ID,
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

func protoToRoleName(r adminv1.Role) string {
	switch r {
	case adminv1.Role_ROLE_OPERATOR:
		return "OPERATOR"
	case adminv1.Role_ROLE_QC_SUPERVISOR:
		return "QC_SUPERVISOR"
	case adminv1.Role_ROLE_WAREHOUSE_STAFF:
		return "WAREHOUSE_STAFF"
	case adminv1.Role_ROLE_MANAGER:
		return "MANAGER"
	case adminv1.Role_ROLE_ADMIN:
		return "ADMIN"
	default:
		return ""
	}
}

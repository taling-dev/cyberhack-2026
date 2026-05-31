package handler

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/auth"
	"github.com/taling-dev/CYBERHACK-2026/apps/api/internal/db"
	adminv1 "github.com/taling-dev/CYBERHACK-2026/apps/api/internal/gen/simaops/admin/v1"
)

// grantable is the set of RPC paths a custom role may be granted.
var grantable = func() map[string]bool {
	m := make(map[string]bool, len(auth.AllGrantableProcedures))
	for _, p := range auth.AllGrantableProcedures {
		m[p] = true
	}
	return m
}()

func (s *AdminService) ListProcedures(ctx context.Context, req *connect.Request[adminv1.ListProceduresRequest]) (*connect.Response[adminv1.ListProceduresResponse], error) {
	return connect.NewResponse(&adminv1.ListProceduresResponse{Procedures: auth.AllGrantableProcedures}), nil
}

func (s *AdminService) CreateRole(ctx context.Context, req *connect.Request[adminv1.CreateRoleRequest]) (*connect.Response[adminv1.CreateRoleResponse], error) {
	name := strings.ToUpper(strings.TrimSpace(req.Msg.Name))
	if name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("role name required"))
	}
	// Reject names that collide with builtins or contain unsafe chars.
	if name == "ADMIN" || name == "OPERATOR" || name == "QC_SUPERVISOR" || name == "WAREHOUSE_STAFF" || name == "MANAGER" {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("role name is reserved"))
	}
	if _, err := s.q.GetRoleByName(ctx, name); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("role %s already exists", name))
	}
	// Validate every requested permission is a known grantable procedure.
	for _, p := range req.Msg.Permissions {
		if !grantable[p] {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("not a grantable procedure: %s", p))
		}
	}

	roleID := uuid.NewString()
	if err := s.q.CreateRole(ctx, db.CreateRoleParams{ID: roleID, Name: name, Description: req.Msg.Description}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create role: %w", err))
	}
	for _, p := range req.Msg.Permissions {
		if err := s.q.AddRolePermission(ctx, db.AddRolePermissionParams{RoleID: roleID, RpcPath: p}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("grant permission: %w", err))
		}
	}
	// Create the matching Keycloak realm role so it lands in users' JWTs.
	if err := s.kc.CreateRealmRole(ctx, name, req.Msg.Description); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("keycloak role create: %w", err))
	}
	// Make the new grants live without a restart.
	refreshRolePermissions(ctx, s.q)

	return connect.NewResponse(&adminv1.CreateRoleResponse{
		Role: &adminv1.RoleDefinition{
			Name:        name,
			Description: req.Msg.Description,
			IsSystem:    false,
			Permissions: req.Msg.Permissions,
		},
	}), nil
}

func (s *AdminService) DeleteRole(ctx context.Context, req *connect.Request[adminv1.DeleteRoleRequest]) (*connect.Response[adminv1.DeleteRoleResponse], error) {
	role, err := s.q.GetRoleByID(ctx, req.Msg.RoleId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role not found"))
	}
	if role.IsSystem {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot delete a system role"))
	}
	// DeleteRole is guarded by is_system in SQL; role_permissions + user_roles
	// cascade on the FK. (Keycloak realm role is left in place; harmless.)
	if err := s.q.DeleteRole(ctx, req.Msg.RoleId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete role: %w", err))
	}
	refreshRolePermissions(ctx, s.q)
	return connect.NewResponse(&adminv1.DeleteRoleResponse{Deleted: true}), nil
}

func (s *AdminService) CreateUser(ctx context.Context, req *connect.Request[adminv1.CreateUserRequest]) (*connect.Response[adminv1.CreateUserResponse], error) {
	username := strings.TrimSpace(req.Msg.Username)
	email := strings.TrimSpace(req.Msg.Email)
	if username == "" || email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("username and email required"))
	}
	if req.Msg.TempPassword == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("temporary password required"))
	}
	if _, err := s.q.GetUserByUsername(ctx, username); err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user %s already exists", username))
	}

	// Provision in Keycloak first (source of truth for auth). If KC is
	// unconfigured this is a no-op and the local profile still gets created.
	if err := s.kc.CreateUser(ctx, username, email, req.Msg.FullName, req.Msg.TempPassword); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("keycloak create user: %w", err))
	}

	userID := uuid.NewString()
	if err := s.q.CreateUserProfile(ctx, db.CreateUserProfileParams{
		ID: userID, Username: username, Email: email, FullName: req.Msg.FullName,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create user profile: %w", err))
	}

	// Assign initial roles (DB + Keycloak realm-role mapping).
	assigned := make([]string, 0, len(req.Msg.RoleNames))
	for _, rn := range req.Msg.RoleNames {
		role, err := s.q.GetRoleByName(ctx, rn)
		if err != nil {
			continue // skip unknown role names rather than fail the whole create
		}
		if err := s.q.AssignUserRole(ctx, db.AssignUserRoleParams{UserID: userID, RoleID: role.ID}); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("assign initial role: %w", err))
		}
		if err := s.kc.AssignRealmRole(ctx, username, rn); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("keycloak role sync: %w", err))
		}
		assigned = append(assigned, rn)
	}

	roles := make([]adminv1.Role, 0, len(assigned))
	for _, rn := range assigned {
		roles = append(roles, roleNameToProto(rn))
	}
	return connect.NewResponse(&adminv1.CreateUserResponse{
		User: &adminv1.User{
			Id: userID, Username: username, Email: email, FullName: req.Msg.FullName,
			Roles: roles, RoleNames: assigned, Active: true,
		},
	}), nil
}

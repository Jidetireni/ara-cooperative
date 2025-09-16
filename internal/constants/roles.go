package constants

import (
	"database/sql"
	_ "embed"
	"encoding/json"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
)

type UserPermissions string

const (
	MemberWriteALLPermission UserPermissions = "member:write:all"
	MemberReadALLPermission  UserPermissions = "member:read:all"
	MemberWriteOwnPermission UserPermissions = "member:write:own"
	MemberReadOwnPermission  UserPermissions = "member:read:own"

	LoanApplyPermission   UserPermissions = "loan:apply"
	LoanApprovePermission UserPermissions = "loan:approve"

	LedgerReadALLPermission UserPermissions = "ledger:read:all"
	LedgerReadOwnPermission UserPermissions = "ledger:read:own"

	RoleAssignPermission UserPermissions = "role:assign"
)

type jsonRole struct {
	Permission  string `json:"permission"`
	Description string `json:"description"`
}

//go:embed data/permissions.json
var rolesJSON []byte

var Roles []repository.Role

func IsValidUserPermission(permission string) bool {
	switch UserPermissions(permission) {
	case MemberWriteALLPermission,
		MemberReadALLPermission,
		MemberWriteOwnPermission,
		MemberReadOwnPermission,
		LoanApplyPermission,
		LoanApprovePermission,
		LedgerReadALLPermission,
		LedgerReadOwnPermission,
		RoleAssignPermission:
		return true
	default:
		return false

	}

}

func init() {
	var jsonRoles []jsonRole
	if err := json.Unmarshal(rolesJSON, &jsonRoles); err != nil {
		panic("failed to unmarshal roles JSON: " + err.Error())
	}

	Roles = make([]repository.Role, len(jsonRoles))
	for i, role := range jsonRoles {
		if !IsValidUserPermission(role.Permission) {
			panic("invalid user permission: " + role.Permission)
		}
		Roles[i] = repository.Role{
			Permission: role.Permission,
			Description: sql.NullString{
				String: role.Description,
				Valid:  role.Description != "",
			},
		}
	}
}

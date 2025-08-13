package constants

import (
	"database/sql"
	_ "embed"
	"encoding/json"

	"github.com/Jidetireni/ara-cooperative.git/internal/repository"
)

type UserPermmisions string

const (
	MemberWritePermission    UserPermmisions = "member:write"
	MemberReadPermission     UserPermmisions = "member:read"
	MemberWriteOwnPermission UserPermmisions = "member:write:own"
	MemberReadOwnPermission  UserPermmisions = "member:read:own"

	LoanApplyPermission   UserPermmisions = "loan:apply"
	LoanApprovePermission UserPermmisions = "loan:approve"

	LedgerReadPermission    UserPermmisions = "ledger:read"
	LedgerReadOwnPermission UserPermmisions = "ledger:read:own"

	RoleAssignPermission UserPermmisions = "role:assign"
)

type jsonRole struct {
	Permission  string `json:"permission"`
	Description string `json:"description"`
}

//go:embed data/permissions.json
var rolesJSON []byte

var Roles []repository.Role

func IsValidUserPermission(permission string) bool {
	switch UserPermmisions(permission) {
	case MemberWritePermission,
		MemberReadPermission,
		MemberWriteOwnPermission,
		MemberReadOwnPermission,
		LoanApplyPermission,
		LoanApprovePermission,
		LedgerReadPermission,
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

package constants

import (
	"database/sql"
	_ "embed"
	"encoding/json"

	"github.com/Jidetireni/ara-cooperative/internal/repository"
)

type UserPermissions string

const (
	MemberWriteALL UserPermissions = "member:write:all"
	MemberReadALL  UserPermissions = "member:read:all"
	LoanApply      UserPermissions = "loan:apply"
	LoanApprove    UserPermissions = "loan:approve"
	LedgerReadALL  UserPermissions = "ledger:read:all"
	RoleAssign     UserPermissions = "role:assign"
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

var RolePermissions = map[string][]UserPermissions{
	RoleAdmin: {
		MemberWriteALL,
		MemberReadALL,
		LoanApply,
		LoanApprove,
		LedgerReadALL,
		RoleAssign,
	},
	RoleMember: {
		MemberReadALL,
		LoanApply,
	},
}

type jsonRole struct {
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

//go:embed data/permissions.json
var permissionsJSON []byte

var Permissions []repository.Permission

func IsValidUserPermission(permission string) bool {
	switch UserPermissions(permission) {
	case MemberWriteALL,
		MemberReadALL,
		LoanApply,
		LoanApprove,
		LedgerReadALL,
		RoleAssign:
		return true
	default:
		return false
	}

}

func init() {
	var jsonPermissions []jsonRole
	if err := json.Unmarshal(permissionsJSON, &jsonPermissions); err != nil {
		panic("failed to unmarshal permissions JSON: " + err.Error())
	}

	Permissions = make([]repository.Permission, len(jsonPermissions))
	for i, permission := range jsonPermissions {
		if !IsValidUserPermission(permission.Slug) {
			panic("invalid user permission: " + permission.Slug)
		}
		Permissions[i] = repository.Permission{
			Slug: permission.Slug,
			Description: sql.NullString{
				String: permission.Description,
				Valid:  permission.Description != "",
			},
		}
	}
}

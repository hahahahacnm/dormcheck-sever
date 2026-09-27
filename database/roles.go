package database

const (
	RoleAdmin      = 0
	RoleUser       = 1
	RoleSponsor    = 2
	RoleSuperAdmin = 3
)

func IsAdminRole(role int) bool {
	return role == RoleAdmin || role == RoleSuperAdmin
}

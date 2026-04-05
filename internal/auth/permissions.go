package auth

import "strings"

const (
	PermissionRead  = "read"
	PermissionWrite = "write"
	PermissionAdmin = "admin"
)

// HasReadPermission reports whether permissions grant read access.
func HasReadPermission(permissions []string) bool {
	read, _, _ := permissionCapabilities(permissions)
	return read
}

// HasWritePermission reports whether permissions grant write access.
func HasWritePermission(permissions []string) bool {
	_, write, _ := permissionCapabilities(permissions)
	return write
}

func permissionCapabilities(permissions []string) (bool, bool, bool) {
	read := false
	write := false
	admin := false
	for _, p := range permissions {
		switch {
		case p == PermissionAdmin:
			admin = true
			write = true
			read = true
		case p == PermissionWrite:
			write = true
			read = true
		case p == PermissionRead:
			read = true
		case strings.HasSuffix(p, ":write"):
			write = true
			read = true
		case strings.HasSuffix(p, ":read"):
			read = true
		}
	}
	return read, write, admin
}

package api

import "strings"

type Role string

const (
	KeibiAdminRole Role = "keibi-admin"
	AdminRole      Role = "admin"
	EditorRole     Role = "editor"
	ViewerRole     Role = "viewer"
)

func GetRole(s string) Role {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case string(KeibiAdminRole):
		return KeibiAdminRole
	case string(AdminRole):
		return AdminRole
	case string(EditorRole):
		return EditorRole
	case string(ViewerRole):
		return ViewerRole
	default:
		return ""
	}

}

package api

import "strings"

type Role string

const (
	KaytuAdminRole Role = "kaytu-admin"
	AdminRole      Role = "admin"
	EditorRole     Role = "editor"
	ViewerRole     Role = "viewer"
)

func GetRole(s string) Role {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case string(KaytuAdminRole):
		return KaytuAdminRole
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

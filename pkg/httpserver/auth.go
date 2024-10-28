package httpserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/opengovern/og-util/pkg/api"
)

const (
	XPlatformUserIDHeader         = "X-Platform-UserId"
	XPlatformUserRoleHeader       = "X-Platform-UserRole"
	XPlatformUserConnectionsScope = "X-Platform-UserConnectionsScope"
)

func AuthorizeHandler(h echo.HandlerFunc, minRole api.Role) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		if err := RequireMinRole(ctx, minRole); err != nil {
			return err
		}

		return h(ctx)
	}
}

func RequireMinRole(ctx echo.Context, minRole api.Role) error {
	if !hasAccess(GetUserRole(ctx), minRole) {
		userRole := GetUserRole(ctx)
		fmt.Println("required role = ", minRole, " user role = ", userRole)
		return echo.NewHTTPError(http.StatusNotAcceptable, "missing required permission")
	}

	return nil
}

func GetUserRole(ctx echo.Context) api.Role {
	role := ctx.Request().Header.Get(XPlatformUserRoleHeader)
	if strings.TrimSpace(role) == "" {
		panic(fmt.Errorf("header %s is missing", XPlatformUserRoleHeader))
	}

	return api.GetRole(role)
}

func GetUserID(ctx echo.Context) string {
	id := ctx.Request().Header.Get(XPlatformUserIDHeader)
	if strings.TrimSpace(id) == "" {
		panic(fmt.Errorf("header %s is missing", XPlatformUserIDHeader))
	}

	return id
}

func CheckAccessToConnectionID(ctx echo.Context, connectionID string) error {
	connectionIDsStr := ctx.Request().Header.Get(XPlatformUserConnectionsScope)
	if len(connectionIDsStr) == 0 {
		return nil
	}

	arr := strings.Split(connectionIDsStr, ",")
	if len(arr) == 0 {
		return nil
	}

	for _, item := range arr {
		if item == connectionID {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusNotAcceptable, "Invalid connection ID")
}

func ResolveConnectionIDs(ctx echo.Context, connectionIDs []string) ([]string, error) {
	connectionIDsStr := ctx.Request().Header.Get(XPlatformUserConnectionsScope)
	if len(connectionIDsStr) == 0 {
		return connectionIDs, nil
	}

	arr := strings.Split(connectionIDsStr, ",")
	if len(arr) == 0 {
		return connectionIDs, nil
	}

	if len(connectionIDs) == 0 {
		return arr, nil
	} else {
		var res []string
		for _, connID := range connectionIDs {
			allowed := false
			for _, item := range arr {
				if item == connID {
					allowed = true
				}
			}

			if allowed {
				res = append(res, connID)
			}
		}
		if len(res) == 0 {
			return nil, echo.NewHTTPError(http.StatusForbidden, "invalid connection ids")
		}
		return res, nil
	}
}

func roleToPriority(role api.Role) int {
	switch role {
	case api.ViewerRole:
		return 0
	case api.EditorRole:
		return 1
	case api.AdminRole:
		return 2
	default:
		panic("unsupported role: " + role)
	}
}

func hasAccess(currRole, minRole api.Role) bool {
	return roleToPriority(currRole) >= roleToPriority(minRole)
}

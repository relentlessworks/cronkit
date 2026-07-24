package api

import (
	"net/http"
	"strings"

	"github.com/relentlessworks/cronkit/internal/auth"
)

// Middleware wraps the auth middleware.
type Middleware struct {
	auth *auth.Auth
}

// NewMiddleware creates a new middleware instance.
func NewMiddleware(a *auth.Auth) *Middleware {
	return &Middleware{auth: a}
}

// RequireAuth middleware checks for a valid bearer token.
func (m *Middleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" {
			writeError(w, r, http.StatusUnauthorized,
				"missing auth token",
				"call POST /auth/request with email to get an OTP, then POST /auth/verify to get a bearer token. Send it as: Authorization: Bearer <token>")
			return
		}

		email, err := m.auth.ValidateToken(token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized,
				"invalid or expired token",
				"request a new token via POST /auth/request with email, then POST /auth/verify")
			return
		}

		workspace, err := m.auth.GetWorkspaceFromToken(token)
		if err != nil || workspace == "" {
			writeError(w, r, http.StatusForbidden,
				"no workspace associated with token",
				"create a workspace first: POST /workspaces with name=<name>")
			return
		}

		// Check workspace still exists
		ws, ok := m.auth.Store().GetWorkspace(workspace)
		if !ok {
			writeError(w, r, http.StatusForbidden,
				"workspace not found",
				"create a new workspace: POST /workspaces with name=<name>")
			return
		}

		r.Header.Set("X-Email", email)
		r.Header.Set("X-Workspace", ws.Handle)
		next(w, r)
	}
}

func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	parts := strings.SplitN(auth, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

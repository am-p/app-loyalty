package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"clientesFrecuentes/internal/auth"
	"clientesFrecuentes/internal/web"

	"github.com/gin-gonic/gin"
)

const ActorKey = "actor"

// Actor is the authenticated identity after checking current account state.
// It does not grant access to a brand: resource membership is checked separately.
type Actor struct {
	ID          int64
	AccountType string
	SessionID   string
	AuthTime    time.Time
}

type ActiveActorStore interface {
	ActiveSessionAccountType(ctx context.Context, userID int64, sessionID string, authVersion int) (string, error)
}

func RequireAuth(tokens *auth.Tokens, actors ActiveActorStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !found || raw == "" {
			unauthenticated(c)
			return
		}
		userID, accountType, sessionID, authVersion, authTime, err := tokens.ParseSessionContext(raw)
		if err != nil {
			unauthenticated(c)
			return
		}
		// JWT claims identify the candidate actor, but current database state is
		// authoritative for every request. This makes suspension immediate without
		// waiting for a previously issued token to expire.
		if actors == nil {
			unauthenticated(c)
			return
		}
		activeAccountType, err := actors.ActiveSessionAccountType(c.Request.Context(), userID, sessionID, authVersion)
		if err != nil || activeAccountType != accountType {
			unauthenticated(c)
			return
		}
		c.Set(ActorKey, Actor{ID: userID, AccountType: activeAccountType, SessionID: sessionID, AuthTime: authTime})
		c.Next()
	}
}

func unauthenticated(c *gin.Context) {
	web.AbortError(c, http.StatusUnauthorized, "UNAUTHENTICATED", "Sesión inválida o expirada", nil)
}

func CurrentActor(c *gin.Context) (Actor, bool) {
	v, ok := c.Get(ActorKey)
	if !ok {
		return Actor{}, false
	}
	a, ok := v.(Actor)
	return a, ok
}

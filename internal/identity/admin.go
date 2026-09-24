package identity

import (
	"context"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
)

func (s *Service) ListUsers(ctx context.Context) ([]db.ListUsersRow, error) {
	return s.store.ListUsers(ctx)
}

type UserChange struct {
	IsActive *bool
	IsAdmin  *bool
}

// UpdateUser activates, deactivates, promotes or demotes another user. An
// admin cannot change themselves (so nobody locks themselves out by
// accident), and the last active admin cannot be demoted or deactivated.
func (s *Service) UpdateUser(ctx context.Context, admin db.User, userID int64, ch UserChange) (db.User, error) {
	if userID == admin.ID {
		return db.User{}, ledger.Forbidden("self", "you cannot change your own account here")
	}
	u, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return db.User{}, ledger.Translate(err, "user")
	}
	losesAdmin := u.IsAdmin && u.IsActive &&
		((ch.IsAdmin != nil && !*ch.IsAdmin) || (ch.IsActive != nil && !*ch.IsActive))
	var out db.User
	err = s.store.WithTx(ctx, admin.ID, func(q *db.Queries) error {
		if losesAdmin {
			n, err := q.CountAdmins(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return ledger.Conflict("last_admin", "the last administrator cannot be demoted or deactivated")
			}
		}
		if ch.IsAdmin != nil {
			if err := q.SetUserAdmin(ctx, db.SetUserAdminParams{ID: u.ID, IsAdmin: *ch.IsAdmin}); err != nil {
				return err
			}
		}
		if ch.IsActive != nil {
			if err := q.SetUserActive(ctx, db.SetUserActiveParams{ID: u.ID, IsActive: *ch.IsActive}); err != nil {
				return err
			}
			if !*ch.IsActive {
				if err := q.DeleteUserSessions(ctx, u.ID); err != nil {
					return err
				}
			}
		}
		var err error
		out, err = q.GetUserByID(ctx, u.ID)
		return err
	})
	if err != nil {
		return db.User{}, ledger.Translate(err, "user")
	}
	detail := map[string]any{"by": admin.Username}
	if ch.IsActive != nil {
		detail["is_active"] = *ch.IsActive
	}
	if ch.IsAdmin != nil {
		detail["is_admin"] = *ch.IsAdmin
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, Name: "admin_user_changed", Detail: detail})
	return out, nil
}

func (s *Service) ListEvents(ctx context.Context, limit int32) ([]db.ListAuthEventsRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.store.ListAuthEvents(ctx, limit)
}

// --- the signed-in user's own sessions and sign-in history ---

func (s *Service) Sessions(ctx context.Context, userID int64) ([]db.ListUserSessionsRow, error) {
	return s.store.ListUserSessions(ctx, userID)
}

func (s *Service) RevokeSession(ctx context.Context, u db.User, sessionID int64, c auth.Client) error {
	n, err := s.store.DeleteUserSession(ctx, db.DeleteUserSessionParams{ID: sessionID, UserID: u.ID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ledger.NotFound("session")
	}
	s.record(ctx, auth.Event{Username: u.Username, UserID: &u.ID, IP: c.IP, UserAgent: c.UserAgent,
		Name: "session_revoked", Detail: map[string]any{"session": sessionID}})
	return nil
}

func (s *Service) MyEvents(ctx context.Context, userID int64, limit int32) ([]db.ListUserAuthEventsRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.store.ListUserAuthEvents(ctx, db.ListUserAuthEventsParams{UserID: &userID, Lim: limit})
}

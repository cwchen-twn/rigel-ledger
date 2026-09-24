package routes

import (
	"net/http"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/connections"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// Settings -> Connections: a person links their own institutions. The
// browser seals the credentials to the runner's key before they are sent
// (web/src/lib/seal.ts); nothing here can read them, and no response ever
// carries them back.

type SealingKeyDTO struct {
	ID        int64  `json:"id"`
	PublicKey []byte `json:"public_key" swaggertype:"string"`
}

type CatalogDTO struct {
	Connectors []ConnectorDTO `json:"connectors"`
	// What to seal to; null while no runner has registered.
	Key *SealingKeyDTO `json:"key"`
}

// connectorCatalog
//
//	@Summary	What can be connected, and the key to seal credentials to
//	@Tags		connections
//	@Produce	json
//	@Success	200	{object}	CatalogDTO
//	@Router		/api/connectors [get]
func (h *handlers) connectorCatalog(w http.ResponseWriter, r *http.Request) {
	cs, key, err := h.conns.Catalog(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := CatalogDTO{Connectors: []ConnectorDTO{}}
	for _, c := range cs {
		out.Connectors = append(out.Connectors, connectorDTO(c))
	}
	if key != nil {
		out.Key = &SealingKeyDTO{ID: key.ID, PublicKey: key.PublicKey}
	}
	response.JSON(w, http.StatusOK, out)
}

type ChallengeDTO struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind" enums:"otp,captcha,device"`
	Prompt    string `json:"prompt"`
	Image     []byte `json:"image,omitempty" swaggertype:"string"`
	ExpiresAt string `json:"expires_at"`
}

type ConnectionDTO struct {
	ID            int64   `json:"id"`
	BookID        int64   `json:"book_id"`
	BookName      string  `json:"book_name"`
	Connector     string  `json:"connector"`
	Label         string  `json:"label"`
	Enabled       bool    `json:"enabled"`
	IntervalHours int16   `json:"interval_hours"`
	Status        string  `json:"status" enums:"new,ok,needs_user_action,failed"`
	LastError     string  `json:"last_error"`
	LastRunAt     *string `json:"last_run_at"`
	RunRequested  bool    `json:"run_requested"`
	// The runner's key changed since these credentials were sealed: enter them again.
	KeyRetired bool          `json:"key_retired"`
	Challenge  *ChallengeDTO `json:"challenge"`
}

// listConnections
//
//	@Summary	Your connections (never with their credentials)
//	@Tags		connections
//	@Produce	json
//	@Success	200	{array}	ConnectionDTO
//	@Router		/api/me/connections [get]
func (h *handlers) listConnections(w http.ResponseWriter, r *http.Request) {
	id, _ := auth.FromContext(r.Context())
	rows, err := h.conns.List(r.Context(), id.User.ID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]ConnectionDTO, len(rows))
	for i, c := range rows {
		out[i] = ConnectionDTO{ID: c.ID, BookID: c.BookID, BookName: c.BookName, Connector: c.Connector, Label: c.Label,
			Enabled: c.Enabled, IntervalHours: c.IntervalHours, Status: c.Status, LastError: c.LastError,
			LastRunAt: timestampPtr(c.LastRunAt), KeyRetired: c.KeyRetired,
			RunRequested: c.RunRequestedAt != nil && (c.LastRunAt == nil || c.RunRequestedAt.After(*c.LastRunAt))}
		if c.ChallengeID != 0 {
			out[i].Challenge = &ChallengeDTO{ID: c.ChallengeID, Kind: c.ChallengeKind, Prompt: c.ChallengePrompt,
				Image: c.ChallengeImage, ExpiresAt: timestamp(c.ChallengeExpiresAt)}
		}
	}
	response.JSON(w, http.StatusOK, out)
}

type ConnectionInputDTO struct {
	BookID        int64  `json:"book_id"`
	Connector     string `json:"connector"`
	Label         string `json:"label"`
	IntervalHours int    `json:"interval_hours"`
	// The runner key the credentials were sealed to (from GET /api/connectors).
	KeyID int64 `json:"key_id"`
	// The credentials as a JSON object of the connector's fields, sealed in
	// the browser with additional data "rigel-ledger/credentials/v1\0<your user id>\0<connector>".
	Sealed []byte `json:"sealed" swaggertype:"string"`
}

type CreatedDTO struct {
	ID int64 `json:"id"`
}

func (h *handlers) auditConnection(r *http.Request, name string, id int64) {
	u, _ := auth.FromContext(r.Context())
	c := clientOf(r)
	if err := h.auth.Record(r.Context(), auth.Event{Username: u.User.Username, UserID: &u.User.ID, IP: c.IP,
		UserAgent: c.UserAgent, Name: name, Detail: map[string]any{"connection_id": id}}); err != nil {
		h.logger.Warn("auth event not recorded", "event", name, "error", err)
	}
}

// createConnection
//
//	@Summary	Link an institution
//	@Tags		connections
//	@Accept		json
//	@Produce	json
//	@Param		body	body		ConnectionInputDTO	true	"connection"
//	@Success	201		{object}	CreatedDTO
//	@Router		/api/me/connections [post]
func (h *handlers) createConnection(w http.ResponseWriter, r *http.Request) {
	var req ConnectionInputDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	cid, err := h.conns.Create(r.Context(), id.User.ID, connections.Input{BookID: req.BookID, Connector: req.Connector,
		Label: req.Label, IntervalHours: req.IntervalHours, KeyID: req.KeyID, Sealed: req.Sealed})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditConnection(r, "connection_created", cid)
	response.JSON(w, http.StatusCreated, CreatedDTO{ID: cid})
}

type CredentialsDTO struct {
	KeyID  int64  `json:"key_id"`
	Sealed []byte `json:"sealed" swaggertype:"string"`
}

// replaceCredentials
//
//	@Summary	Enter a connection's credentials again
//	@Tags		connections
//	@Accept		json
//	@Param		connectionID	path	int				true	"connection id"
//	@Param		body			body	CredentialsDTO	true	"sealed credentials"
//	@Success	204
//	@Router		/api/me/connections/{connectionID}/credentials [put]
func (h *handlers) replaceCredentials(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	var req CredentialsDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.conns.ReplaceCredentials(r.Context(), id.User.ID, cid, req.KeyID, req.Sealed); err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditConnection(r, "connection_credentials_replaced", cid)
	response.NoContent(w)
}

type ConnectionUpdateDTO struct {
	BookID        *int64  `json:"book_id"`
	Label         *string `json:"label"`
	Enabled       *bool   `json:"enabled"`
	IntervalHours *int    `json:"interval_hours"`
}

// updateConnection
//
//	@Summary	Change a connection's book, label, schedule or on/off
//	@Tags		connections
//	@Accept		json
//	@Param		connectionID	path	int					true	"connection id"
//	@Param		body			body	ConnectionUpdateDTO	true	"changes"
//	@Success	204
//	@Router		/api/me/connections/{connectionID} [patch]
func (h *handlers) updateConnection(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	var req ConnectionUpdateDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.conns.Update(r.Context(), id.User.ID, cid, connections.Update{BookID: req.BookID, Label: req.Label,
		Enabled: req.Enabled, IntervalHours: req.IntervalHours}); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// syncConnection
//
//	@Summary	Sync a connection at the runner's next poll
//	@Tags		connections
//	@Param		connectionID	path	int	true	"connection id"
//	@Success	204
//	@Router		/api/me/connections/{connectionID}/sync [post]
func (h *handlers) syncConnection(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.conns.RunNow(r.Context(), id.User.ID, cid); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// deleteConnection
//
//	@Summary	Unlink an institution (its sealed credentials are deleted)
//	@Tags		connections
//	@Param		connectionID	path	int	true	"connection id"
//	@Success	204
//	@Router		/api/me/connections/{connectionID} [delete]
func (h *handlers) deleteConnection(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.conns.Delete(r.Context(), id.User.ID, cid); err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditConnection(r, "connection_deleted", cid)
	response.NoContent(w)
}

type ChallengeReplyDTO struct {
	// The answer, sealed with additional data "rigel-ledger/answer/v1\0<challenge id>".
	Sealed []byte `json:"sealed" swaggertype:"string"`
}

// answerChallenge
//
//	@Summary	Answer the runner's OTP, CAPTCHA or device check
//	@Tags		connections
//	@Accept		json
//	@Param		connectionID	path	int					true	"connection id"
//	@Param		challengeID		path	int					true	"challenge id"
//	@Param		body			body	ChallengeReplyDTO	true	"sealed answer"
//	@Success	204
//	@Router		/api/me/connections/{connectionID}/challenges/{challengeID} [post]
func (h *handlers) answerChallenge(w http.ResponseWriter, r *http.Request) {
	cid, ok := pathID(r, "connectionID")
	chid, ok2 := pathID(r, "challengeID")
	if !ok || !ok2 {
		badParam(w, "challengeID", "invalid")
		return
	}
	var req ChallengeReplyDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	if err := h.conns.AnswerChallenge(r.Context(), id.User.ID, cid, chid, req.Sealed); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

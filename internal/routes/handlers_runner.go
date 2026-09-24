package routes

import (
	"net/http"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/auth"
	"github.com/cwchen-twn/rigel-ledger/internal/connections"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

// The sync runner's API (/api/runner/*, a runner token only). The runner
// registers its keys and connectors, claims due connections, opens their
// sealed credentials itself, and reports back: rows, challenges, the end
// of the run. The app never sees what the blobs hold.

type RunnerKeysDTO struct {
	// Every key the runner holds, oldest first, the one to seal to last
	// (raw 32-byte X25519 public keys, base64).
	PublicKeys [][]byte `json:"public_keys" swaggertype:"array,string"`
}

type RunnerKeyDTO struct {
	ID        int64   `json:"id"`
	PublicKey []byte  `json:"public_key" swaggertype:"string"`
	CreatedAt string  `json:"created_at"`
	RetiredAt *string `json:"retired_at"`
}

// runnerKeys
//
//	@Summary	Register the runner's public keys
//	@Description	Keys left out are retired; connections sealed to them wait for new credentials.
//	@Tags		runner
//	@Accept		json
//	@Produce	json
//	@Param		body	body	RunnerKeysDTO	true	"keys"
//	@Success	200		{array}	RunnerKeyDTO
//	@Router		/api/runner/keys [post]
func (h *handlers) runnerKeys(w http.ResponseWriter, r *http.Request) {
	var req RunnerKeysDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	keys, err := h.conns.RegisterKeys(r.Context(), req.PublicKeys)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]RunnerKeyDTO, len(keys))
	for i, k := range keys {
		out[i] = RunnerKeyDTO{ID: k.ID, PublicKey: k.PublicKey, CreatedAt: timestamp(k.CreatedAt), RetiredAt: timestampPtr(k.RetiredAt)}
	}
	response.JSON(w, http.StatusOK, out)
}

type ConnectorFieldDTO struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Kind     string `json:"kind" enums:"text,secret,id_number"`
	Optional bool   `json:"optional,omitempty"`
}

type ConnectorDTO struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Country string              `json:"country"`
	Fields  []ConnectorFieldDTO `json:"fields"`
}

type PublishConnectorsDTO struct {
	Connectors []ConnectorDTO `json:"connectors"`
}

// publishConnectors
//
//	@Summary	Publish what the runner can connect to
//	@Tags		runner
//	@Accept		json
//	@Param		body	body	PublishConnectorsDTO	true	"connectors"
//	@Success	204
//	@Router		/api/runner/connectors [put]
func (h *handlers) publishConnectors(w http.ResponseWriter, r *http.Request) {
	var req PublishConnectorsDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	cs := make([]connections.Connector, len(req.Connectors))
	for i, c := range req.Connectors {
		cs[i] = connections.Connector{ID: c.ID, Name: c.Name, Country: c.Country}
		for _, f := range c.Fields {
			cs[i].Fields = append(cs[i].Fields, connections.Field{Name: f.Name, Label: f.Label, Kind: f.Kind, Optional: f.Optional})
		}
	}
	if err := h.conns.PublishConnectors(r.Context(), cs); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

type ClaimDTO struct {
	Limit int `json:"limit"`
}

type JobDTO struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	BookID    int64  `json:"book_id"`
	Connector string `json:"connector"`
	KeyID     int64  `json:"key_id"`
	// Open with the key's private half and additional data
	// "rigel-ledger/credentials/v1\0<user_id>\0<connector>" (internal/sealing).
	Sealed []byte `json:"sealed" swaggertype:"string"`
}

// claimJobs
//
//	@Summary	Claim due connections
//	@Description	A claim lapses after 30 minutes. Finish every claimed job.
//	@Tags		runner
//	@Accept		json
//	@Produce	json
//	@Param		body	body	ClaimDTO	true	"how many"
//	@Success	200		{array}	JobDTO
//	@Router		/api/runner/jobs/claim [post]
func (h *handlers) claimJobs(w http.ResponseWriter, r *http.Request) {
	var req ClaimDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	jobs, err := h.conns.Claim(r.Context(), req.Limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]JobDTO, len(jobs))
	for i, j := range jobs {
		out[i] = JobDTO{ID: j.ID, UserID: j.UserID, BookID: j.BookID, Connector: j.Connector, KeyID: j.KeyID, Sealed: j.Sealed}
	}
	response.JSON(w, http.StatusOK, out)
}

// runnerImport
//
//	@Summary	Send a claimed connection's rows to its book's review queue
//	@Description	The same batch as POST /api/books/{bookID}/imports; the connector is the connection's.
//	@Tags		runner
//	@Accept		json
//	@Produce	json
//	@Param		connectionID	path		int				true	"connection id"
//	@Param		body			body		ImportBatchDTO	true	"batch"
//	@Success	201				{object}	ImportResultDTO
//	@Router		/api/runner/connections/{connectionID}/imports [post]
func (h *handlers) runnerImport(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	var req ImportBatchDTO
	if err := response.DecodeLimit(w, r, &req, maxImportBytes); err != nil {
		h.fail(w, r, err)
		return
	}
	res, err := h.conns.Import(r.Context(), id, importInput(req))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, importResultDTO(res))
}

type ChallengeInDTO struct {
	Kind   string `json:"kind" enums:"otp,captcha,device"`
	Prompt string `json:"prompt"`
	// A CAPTCHA image (PNG or JPEG, base64, under 256 KiB).
	Image      []byte `json:"image,omitempty" swaggertype:"string"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type ChallengeCreatedDTO struct {
	ID        int64  `json:"id"`
	ExpiresAt string `json:"expires_at"`
}

// raiseChallenge
//
//	@Summary	Ask the connection's owner for an OTP, a CAPTCHA or a device check
//	@Tags		runner
//	@Accept		json
//	@Produce	json
//	@Param		connectionID	path		int				true	"connection id"
//	@Param		body			body		ChallengeInDTO	true	"challenge"
//	@Success	201				{object}	ChallengeCreatedDTO
//	@Router		/api/runner/connections/{connectionID}/challenges [post]
func (h *handlers) raiseChallenge(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	var req ChallengeInDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	ch, err := h.conns.Challenge(r.Context(), id, connections.ChallengeInput{Kind: req.Kind, Prompt: req.Prompt,
		Image: req.Image, TTL: time.Duration(req.TTLSeconds) * time.Second})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, ChallengeCreatedDTO{ID: ch.ID, ExpiresAt: timestamp(ch.ExpiresAt)})
}

type ChallengeAnswerDTO struct {
	Answered bool `json:"answered"`
	Expired  bool `json:"expired"`
	// Sealed to the runner with additional data "rigel-ledger/answer/v1\0<challenge id>";
	// handed out once.
	Sealed []byte `json:"sealed,omitempty" swaggertype:"string"`
}

// challengeAnswer
//
//	@Summary	Poll a challenge for the owner's answer
//	@Tags		runner
//	@Produce	json
//	@Param		connectionID	path		int	true	"connection id"
//	@Param		challengeID		path		int	true	"challenge id"
//	@Success	200				{object}	ChallengeAnswerDTO
//	@Router		/api/runner/connections/{connectionID}/challenges/{challengeID} [get]
func (h *handlers) challengeAnswer(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "connectionID")
	cid, ok2 := pathID(r, "challengeID")
	if !ok || !ok2 {
		badParam(w, "challengeID", "invalid")
		return
	}
	a, err := h.conns.TakeAnswer(r.Context(), id, cid)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusOK, ChallengeAnswerDTO{Answered: a.Answered, Expired: a.Expired, Sealed: a.Sealed})
}

type FinishDTO struct {
	Status string `json:"status" enums:"ok,failed"`
	// A stable code when failed: bad_credentials, challenge_expired, institution_down, ...
	Error string `json:"error"`
}

// finishRun
//
//	@Summary	End a claimed connection's run
//	@Tags		runner
//	@Accept		json
//	@Param		connectionID	path	int			true	"connection id"
//	@Param		body			body	FinishDTO	true	"outcome"
//	@Success	204
//	@Router		/api/runner/connections/{connectionID}/finish [post]
func (h *handlers) finishRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "connectionID")
	if !ok {
		badParam(w, "connectionID", "invalid")
		return
	}
	var req FinishDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.conns.Finish(r.Context(), id, req.Status, req.Error); err != nil {
		h.fail(w, r, err)
		return
	}
	response.NoContent(w)
}

// ---- Administration -> Sync runner ----

type RunnerTokenDTO struct {
	ID         int64  `json:"id"`
	Label      string `json:"label"`
	CreatedBy  string `json:"created_by"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
	ExpiresAt  string `json:"expires_at"`
}

type RunnerStatusDTO struct {
	Tokens     []RunnerTokenDTO `json:"tokens"`
	Keys       []RunnerKeyDTO   `json:"keys"`
	Connectors []ConnectorDTO   `json:"connectors"`
}

func connectorDTO(c connections.Connector) ConnectorDTO {
	out := ConnectorDTO{ID: c.ID, Name: c.Name, Country: c.Country, Fields: []ConnectorFieldDTO{}}
	for _, f := range c.Fields {
		out.Fields = append(out.Fields, ConnectorFieldDTO{Name: f.Name, Label: f.Label, Kind: f.Kind, Optional: f.Optional})
	}
	return out
}

// runnerStatus
//
//	@Summary	The sync runner: its tokens, keys and connectors
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	RunnerStatusDTO
//	@Router		/api/admin/runner [get]
func (h *handlers) runnerStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tokens, err := h.conns.RunnerTokens(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	keys, err := h.conns.RunnerKeys(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cs, _, err := h.conns.Catalog(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := RunnerStatusDTO{Tokens: []RunnerTokenDTO{}, Keys: []RunnerKeyDTO{}, Connectors: []ConnectorDTO{}}
	for _, t := range tokens {
		out.Tokens = append(out.Tokens, RunnerTokenDTO{ID: t.ID, Label: t.Label, CreatedBy: t.Username, CreatedAt: timestamp(t.CreatedAt),
			LastUsedAt: timestamp(t.LastUsedAt), ExpiresAt: timestamp(t.ExpiresAt)})
	}
	for _, k := range keys {
		out.Keys = append(out.Keys, RunnerKeyDTO{ID: k.ID, PublicKey: k.PublicKey, CreatedAt: timestamp(k.CreatedAt), RetiredAt: timestampPtr(k.RetiredAt)})
	}
	for _, c := range cs {
		out.Connectors = append(out.Connectors, connectorDTO(c))
	}
	response.JSON(w, http.StatusOK, out)
}

// createRunnerToken
//
//	@Summary	Make a token for the sync runner (shown once)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		CreateTokenDTO	true	"label and lifetime"
//	@Success	201		{object}	TokenCreatedDTO
//	@Router		/api/admin/runner/tokens [post]
func (h *handlers) createRunnerToken(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenDTO
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	days, err := tokenDays(req)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	id, _ := auth.FromContext(r.Context())
	token, s, err := h.auth.CreateRunnerToken(r.Context(), id.User, req.Label, time.Duration(days)*24*time.Hour, clientOf(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	response.JSON(w, http.StatusCreated, TokenCreatedDTO{Token: token, Session: sessionDTO(s)})
}

// revokeRunnerToken
//
//	@Summary	Revoke a sync runner token
//	@Tags		admin
//	@Param		sessionID	path	int	true	"token id"
//	@Success	204
//	@Router		/api/admin/runner/tokens/{sessionID} [delete]
func (h *handlers) revokeRunnerToken(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(r, "sessionID")
	if !ok {
		badParam(w, "sessionID", "invalid")
		return
	}
	if err := h.conns.RevokeRunnerToken(r.Context(), sid); err != nil {
		h.fail(w, r, err)
		return
	}
	h.auditAdmin(r, "runner_token_revoked")
	response.NoContent(w)
}

// Package connections is P4c-1: each person links their own institutions
// (Settings -> Connections), and one sync runner works through them as a job
// queue. The credentials are sealed in the browser to the runner's key
// (internal/sealing): this package stores and hands out ciphertext and never
// holds anything that opens it.
package connections

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/ledger"
	"github.com/cwchen-twn/rigel-ledger/internal/sealing"
)

const (
	maxChallengeTTL = 15 * time.Minute
	maxImage        = 256 << 10 // a CAPTCHA, not a photo
)

var (
	idPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)
	fieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	codePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,59}$`)
)

type Service struct {
	store  *db.Store
	ledger *ledger.Service
}

func New(store *db.Store, l *ledger.Service) *Service { return &Service{store: store, ledger: l} }

// CredentialsAAD binds a credentials blob to one person's connector; the
// browser seals with the same (web/src/lib/seal.ts).
func CredentialsAAD(userID int64, connector string) []byte {
	return sealing.AAD("credentials", strconv.FormatInt(userID, 10), connector)
}

// AnswerAAD binds a challenge answer to its challenge.
func AnswerAAD(challengeID int64) []byte {
	return sealing.AAD("answer", strconv.FormatInt(challengeID, 10))
}

// Field is one input a connector needs; the UI renders its form from these.
type Field struct {
	Name     string `json:"name"`
	Label    string `json:"label"` // English default; the UI prefers connector.<id>.field.<name>
	Kind     string `json:"kind"`  // text | secret | id_number
	Optional bool   `json:"optional,omitempty"`
}

type Connector struct {
	ID      string
	Name    string
	Country string
	Fields  []Field
}

func connectorOf(c db.RunnerConnector) Connector {
	out := Connector{ID: c.ID, Name: c.Name, Country: c.Country}
	_ = json.Unmarshal(c.Fields, &out.Fields)
	return out
}

// ---------------------------------------------------------------------------
// Runner side
// ---------------------------------------------------------------------------

// RegisterKeys records the public keys the runner holds, newest last; every
// other key is retired, and connections sealed to it wait for their owner
// to enter the credentials again.
func (s *Service) RegisterKeys(ctx context.Context, keys [][]byte) ([]db.RunnerKey, error) {
	if len(keys) == 0 || len(keys) > 5 {
		return nil, ledger.FieldError("public_keys", "out_of_range", "1 to 5 keys")
	}
	var out []db.RunnerKey
	err := s.store.WithTx(ctx, 0, func(q *db.Queries) error {
		var keep []int64
		for i, k := range keys {
			if !sealing.ValidPublicKey(k) {
				return ledger.FieldError("public_keys["+strconv.Itoa(i)+"]", "invalid", "not an X25519 public key")
			}
			rk, err := q.UpsertRunnerKey(ctx, k)
			if err != nil {
				return err
			}
			keep = append(keep, rk.ID)
			out = append(out, rk)
		}
		return q.RetireRunnerKeysExcept(ctx, keep)
	})
	return out, err
}

// PublishConnectors records what the runner can connect to. A connector it
// stops publishing stays, so existing connections keep their name.
func (s *Service) PublishConnectors(ctx context.Context, cs []Connector) error {
	for i, c := range cs {
		at := "connectors[" + strconv.Itoa(i) + "]"
		if !idPattern.MatchString(c.ID) || strings.TrimSpace(c.Name) == "" {
			return ledger.FieldError(at, "invalid", "a connector needs an id (a-z, 0-9, -, _) and a name")
		}
		if len(c.Fields) == 0 || len(c.Fields) > 10 {
			return ledger.FieldError(at+".fields", "out_of_range", "1 to 10 fields")
		}
		for _, f := range c.Fields {
			if !fieldPattern.MatchString(f.Name) || (f.Kind != "text" && f.Kind != "secret" && f.Kind != "id_number") {
				return ledger.FieldError(at+".fields", "invalid", "a field needs a name and a kind: text, secret or id_number")
			}
		}
	}
	return s.store.WithTx(ctx, 0, func(q *db.Queries) error {
		for _, c := range cs {
			fields, _ := json.Marshal(c.Fields)
			if err := q.UpsertConnector(ctx, db.UpsertConnectorParams{ID: c.ID, Name: strings.TrimSpace(c.Name),
				Country: strings.ToUpper(strings.TrimSpace(c.Country)), Fields: fields}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Claim hands the runner up to limit due connections.
func (s *Service) Claim(ctx context.Context, limit int) ([]db.ClaimJobsRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.store.ClaimJobs(ctx, int32(limit))
}

func (s *Service) claimed(ctx context.Context, id int64) (db.Connection, error) {
	c, err := s.store.GetClaimedConnection(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Connection{}, ledger.Conflict("not_claimed", "claim the job before working on it")
	}
	return c, err
}

// Import stages a batch for a claimed connection, as its owner, into its
// book. The connector is the connection's, whatever the batch says.
func (s *Service) Import(ctx context.Context, id int64, in ledger.ImportInput) (ledger.ImportResult, error) {
	c, err := s.claimed(ctx, id)
	if err != nil {
		return ledger.ImportResult{}, err
	}
	a, err := s.ledger.ResolveAccess(ctx, c.UserID, c.BookID)
	if err != nil {
		return ledger.ImportResult{}, ledger.Forbidden("no_access", "the connection's owner no longer has this book")
	}
	in.Connector = c.Connector
	return s.ledger.Import(ctx, a, in)
}

// Finish ends a run: ok, or failed with a stable code (bad_credentials,
// challenge_expired, institution_down, ...), never the institution's text.
func (s *Service) Finish(ctx context.Context, id int64, status, code string) error {
	if status != "ok" && status != "failed" {
		return ledger.FieldError("status", "invalid", "ok or failed")
	}
	if status == "failed" && !codePattern.MatchString(code) {
		return ledger.FieldError("error", "invalid", "a failure needs a code: a-z, 0-9, _")
	}
	if status == "ok" {
		code = ""
	}
	n, err := s.store.FinishRun(ctx, db.FinishRunParams{ID: id, Status: status, LastError: code})
	if err != nil {
		return err
	}
	if n == 0 {
		return ledger.Conflict("not_claimed", "claim the job before working on it")
	}
	return nil
}

type ChallengeInput struct {
	Kind   string // otp | captcha | device
	Prompt string
	Image  []byte // a CAPTCHA (PNG/JPEG); stays in the cluster
	TTL    time.Duration
}

// Challenge asks the owner for what the runner cannot provide itself. The
// connection shows as needing action until the owner answers.
func (s *Service) Challenge(ctx context.Context, id int64, in ChallengeInput) (db.CreateConnectionChallengeRow, error) {
	if _, err := s.claimed(ctx, id); err != nil {
		return db.CreateConnectionChallengeRow{}, err
	}
	if in.Kind != "otp" && in.Kind != "captcha" && in.Kind != "device" {
		return db.CreateConnectionChallengeRow{}, ledger.FieldError("kind", "invalid", "otp, captcha or device")
	}
	if len(in.Image) > maxImage || (in.Kind == "captcha" && len(in.Image) == 0) {
		return db.CreateConnectionChallengeRow{}, ledger.FieldError("image", "invalid", "a CAPTCHA needs an image under 256 KiB")
	}
	if in.TTL <= 0 || in.TTL > maxChallengeTTL {
		in.TTL = 5 * time.Minute
	}
	var out db.CreateConnectionChallengeRow
	err := s.store.WithTx(ctx, 0, func(q *db.Queries) error {
		var err error
		var img []byte
		if len(in.Image) > 0 {
			img = in.Image
		}
		out, err = q.CreateConnectionChallenge(ctx, db.CreateConnectionChallengeParams{ConnectionID: id, Kind: in.Kind,
			Prompt: strings.TrimSpace(in.Prompt), Image: img, ExpiresAt: time.Now().Add(in.TTL)})
		if err != nil {
			return err
		}
		return q.SetConnectionStatus(ctx, db.SetConnectionStatusParams{ID: id, Status: "needs_user_action"})
	})
	return out, err
}

type Answer struct {
	Sealed   []byte // nil until answered; handed out once
	Answered bool
	Expired  bool
}

// TakeAnswer is the runner polling a challenge.
func (s *Service) TakeAnswer(ctx context.Context, id, challengeID int64) (Answer, error) {
	if _, err := s.claimed(ctx, id); err != nil {
		return Answer{}, err
	}
	r, err := s.store.TakeChallengeAnswer(ctx, db.TakeChallengeAnswerParams{ID: challengeID, ConnectionID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Answer{}, ledger.NotFound("challenge")
	}
	if err != nil {
		return Answer{}, err
	}
	return Answer{Sealed: r.AnswerSealed, Answered: r.AnsweredAt != nil, Expired: r.AnsweredAt == nil && time.Now().After(r.ExpiresAt)}, nil
}

// ---------------------------------------------------------------------------
// User side
// ---------------------------------------------------------------------------

// Catalog is what a person can link, and the key to seal to (nil while no
// runner has registered one).
func (s *Service) Catalog(ctx context.Context) ([]Connector, *db.RunnerKey, error) {
	rows, err := s.store.ListConnectors(ctx)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Connector, len(rows))
	for i, r := range rows {
		out[i] = connectorOf(r)
	}
	k, err := s.store.ActiveRunnerKey(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return out, &k, nil
}

func (s *Service) List(ctx context.Context, userID int64) ([]db.ListUserConnectionsRow, error) {
	return s.store.ListUserConnections(ctx, userID)
}

type Input struct {
	BookID        int64
	Connector     string
	Label         string
	IntervalHours int
	KeyID         int64
	Sealed        []byte
}

// canFeed checks that the person may send rows into the book.
func (s *Service) canFeed(ctx context.Context, userID, bookID int64) error {
	a, err := s.ledger.ResolveAccess(ctx, userID, bookID)
	if err != nil {
		return ledger.FieldError("book_id", "not_found", "no such book")
	}
	if !a.Can(db.MemberRoleEditor) {
		return ledger.FieldError("book_id", "role", "you need to be an editor of the book")
	}
	return nil
}

// checkSealed accepts only a blob sealed to the current key. That is all
// the app can know about it: it cannot open it.
func (s *Service) checkSealed(ctx context.Context, keyID int64, blob []byte) error {
	k, err := s.store.ActiveRunnerKey(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return ledger.Invalid("no_runner", "no sync runner has registered yet")
	}
	if err != nil {
		return err
	}
	if keyID != k.ID {
		return ledger.FieldError("key_id", "stale", "the runner's key changed; reload and enter the credentials again")
	}
	if !sealing.WellFormed(blob) {
		return ledger.FieldError("sealed", "invalid", "not a sealed blob")
	}
	return nil
}

func interval(h int) (int16, error) {
	if h == 0 {
		return 24, nil
	}
	if h < 1 || h > 168 {
		return 0, ledger.FieldError("interval_hours", "out_of_range", "1 to 168 hours")
	}
	return int16(h), nil
}

func (s *Service) Create(ctx context.Context, userID int64, in Input) (int64, error) {
	cs, _, err := s.Catalog(ctx)
	if err != nil {
		return 0, err
	}
	known := false
	for _, c := range cs {
		known = known || c.ID == in.Connector
	}
	if !known {
		return 0, ledger.FieldError("connector", "unknown", "the runner does not offer this connector")
	}
	if err := s.canFeed(ctx, userID, in.BookID); err != nil {
		return 0, err
	}
	if err := s.checkSealed(ctx, in.KeyID, in.Sealed); err != nil {
		return 0, err
	}
	iv, err := interval(in.IntervalHours)
	if err != nil {
		return 0, err
	}
	label := strings.TrimSpace(in.Label)
	if len(label) > 80 {
		return 0, ledger.FieldError("label", "too_long", "at most 80 characters")
	}
	var id int64
	err = s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		var err error
		id, err = q.CreateConnection(ctx, db.CreateConnectionParams{UserID: userID, BookID: in.BookID, Connector: in.Connector,
			Label: label, Sealed: in.Sealed, KeyID: in.KeyID, IntervalHours: iv})
		return err
	})
	return id, ledger.Translate(err, "connection")
}

func (s *Service) get(ctx context.Context, userID, id int64) (db.GetUserConnectionRow, error) {
	c, err := s.store.GetUserConnection(ctx, db.GetUserConnectionParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ledger.NotFound("connection")
	}
	return c, err
}

// ReplaceCredentials is the only way to change them: they are never shown.
func (s *Service) ReplaceCredentials(ctx context.Context, userID, id, keyID int64, sealed []byte) error {
	if _, err := s.get(ctx, userID, id); err != nil {
		return err
	}
	if err := s.checkSealed(ctx, keyID, sealed); err != nil {
		return err
	}
	return s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		_, err := q.ReplaceCredentials(ctx, db.ReplaceCredentialsParams{ID: id, UserID: userID, Sealed: sealed, KeyID: keyID})
		return err
	})
}

type Update struct {
	BookID        *int64
	Label         *string
	Enabled       *bool
	IntervalHours *int
}

func (s *Service) Update(ctx context.Context, userID, id int64, u Update) error {
	c, err := s.get(ctx, userID, id)
	if err != nil {
		return err
	}
	p := db.UpdateConnectionParams{ID: id, UserID: userID, BookID: c.BookID, Label: c.Label, Enabled: c.Enabled, IntervalHours: c.IntervalHours}
	if u.BookID != nil && *u.BookID != c.BookID {
		if err := s.canFeed(ctx, userID, *u.BookID); err != nil {
			return err
		}
		p.BookID = *u.BookID
	}
	if u.Label != nil {
		p.Label = strings.TrimSpace(*u.Label)
		if len(p.Label) > 80 {
			return ledger.FieldError("label", "too_long", "at most 80 characters")
		}
	}
	if u.Enabled != nil {
		p.Enabled = *u.Enabled
	}
	if u.IntervalHours != nil {
		if p.IntervalHours, err = interval(*u.IntervalHours); err != nil {
			return err
		}
	}
	return s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		_, err := q.UpdateConnection(ctx, p)
		return err
	})
}

// RunNow asks the runner to sync at its next poll.
func (s *Service) RunNow(ctx context.Context, userID, id int64) error {
	n, err := s.store.RequestRun(ctx, db.RequestRunParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err := s.get(ctx, userID, id); err != nil {
			return err
		}
		return ledger.Invalid("disabled", "turn the connection on first")
	}
	return nil
}

// Delete removes the connection and, with it, the only copy of the sealed
// credentials. Rows it already staged stay in the queue.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.store.WithTx(ctx, userID, func(q *db.Queries) error {
		n, err := q.DeleteConnection(ctx, db.DeleteConnectionParams{ID: id, UserID: userID})
		if err == nil && n == 0 {
			return ledger.NotFound("connection")
		}
		return err
	})
}

// AnswerChallenge stores the owner's answer, sealed to the runner.
func (s *Service) AnswerChallenge(ctx context.Context, userID, id, challengeID int64, sealed []byte) error {
	if !sealing.WellFormed(sealed) {
		return ledger.FieldError("sealed", "invalid", "not a sealed blob")
	}
	n, err := s.store.AnswerChallenge(ctx, db.AnswerChallengeParams{ID: challengeID, ConnectionID: id, UserID: userID, AnswerSealed: sealed})
	if err != nil {
		return err
	}
	if n == 0 {
		return ledger.Conflict("challenge_gone", "this check expired or was already answered")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Administration
// ---------------------------------------------------------------------------

func (s *Service) RunnerTokens(ctx context.Context) ([]db.ListRunnerSessionsRow, error) {
	return s.store.ListRunnerSessions(ctx)
}

func (s *Service) RunnerKeys(ctx context.Context) ([]db.RunnerKey, error) {
	return s.store.ListRunnerKeys(ctx)
}

func (s *Service) RevokeRunnerToken(ctx context.Context, sessionID int64) error {
	n, err := s.store.DeleteRunnerSession(ctx, sessionID)
	if err == nil && n == 0 {
		return ledger.NotFound("token")
	}
	return err
}

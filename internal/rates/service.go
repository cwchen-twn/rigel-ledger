package rates

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
)

const (
	refreshEvery   = 12 * time.Hour // a successful fetch this recent is enough
	checkEvery     = time.Hour
	backfillDays   = 14
	maxDayFailures = 3
	fetchTimeout   = 30 * time.Second
)

type Service struct {
	store     *db.Store
	providers []Provider
	logger    *slog.Logger
	now       func() time.Time
}

func New(store *db.Store, providers []Provider, logger *slog.Logger) *Service {
	return &Service{store: store, providers: providers, logger: logger, now: time.Now}
}

// Result is one fetch as recorded.
type Result = db.RateFetch

// Refresh fetches the latest rates (day nil) or one day's, trying each
// provider in turn, stores them and records the attempt.
func (s *Service) Refresh(ctx context.Context, day *time.Time) (Result, error) {
	var errs []error
	for _, p := range s.providers {
		fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		snap, err := p.Fetch(fctx, day)
		cancel()
		if errors.Is(err, ErrNoHistory) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			s.record(ctx, p.Name(), nil, day, 0, 0, err.Error())
			continue
		}
		stored, skipped, err := s.store1(ctx, snap)
		if err != nil {
			return Result{}, err
		}
		r := s.record(ctx, snap.Source, &snap.Date, day, stored, skipped, "")
		s.logger.Info("Exchange rates stored", "source", snap.Source, "date", snap.Date.Format("2006-01-02"),
			"rates", stored, "skipped_locked", skipped)
		return r, nil
	}
	if len(errs) == 0 {
		return Result{}, errors.New("no provider serves that request")
	}
	return Result{}, errors.Join(errs...)
}

func (s *Service) record(ctx context.Context, source string, date, requested *time.Time, n, skipped int, msg string) Result {
	r, err := s.store.RecordRateFetch(ctx, db.RecordRateFetchParams{
		Source: source, RateDate: date, Requested: requested, Rates: int32(n), Skipped: int32(skipped), Error: msg,
	})
	if err != nil {
		s.logger.Warn("rate fetch not recorded", "error", err)
	}
	return r
}

// store1 upserts USD->X for every ISO currency we know. A rate on or
// before a locked book's date is refused by the prices_lock trigger: that
// one is skipped, the rest go in.
func (s *Service) store1(ctx context.Context, snap Snapshot) (stored, skipped int, err error) {
	currencies, err := s.store.ListCurrencies(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range currencies {
		if c.Code == Base {
			continue
		}
		rate, ok := snap.Rates[c.Code]
		if !ok {
			continue
		}
		_, err := s.store.UpsertPrice(ctx, db.UpsertPriceParams{
			Commodity: Base, Quote: c.Code, Date: snap.Date, Rate: rate, Source: snap.Source,
		})
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.ConstraintName == "book_locked" {
			skipped++
			continue
		}
		if err != nil {
			return stored, skipped, err
		}
		stored++
	}
	return stored, skipped, nil
}

// Tick is one pass of the schedule: the latest rates unless a fetch
// succeeded lately, then any of the last backfillDays days still missing.
func (s *Service) Tick(ctx context.Context) {
	last, err := s.store.LastSuccessfulFetch(ctx)
	if err != nil || s.now().Sub(last.FetchedAt) > refreshEvery {
		if _, err := s.Refresh(ctx, nil); err != nil {
			s.logger.Warn("Exchange-rate refresh failed", "error", err)
		}
	}
	for _, day := range s.missingDays(ctx) {
		if n, _ := s.store.FailedTodayFor(ctx, &day); n >= maxDayFailures {
			continue
		}
		if _, err := s.Refresh(ctx, &day); err != nil {
			s.logger.Warn("Exchange-rate backfill failed", "day", day.Format("2006-01-02"), "error", err)
		}
	}
}

// missingDays are the last backfillDays days (before today, UTC) without a
// successful fetch, oldest first.
func (s *Service) missingDays(ctx context.Context) []time.Time {
	today := s.now().UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -backfillDays)
	have, err := s.store.FetchedDays(ctx, db.FetchedDaysParams{FromDate: &from, ToDate: &today})
	if err != nil {
		return nil
	}
	got := map[string]bool{}
	for _, d := range have {
		got[d.Format("2006-01-02")] = true
	}
	var out []time.Time
	for d := from; d.Before(today); d = d.AddDate(0, 0, 1) {
		if !got[d.Format("2006-01-02")] {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// Run ticks at start (after a short delay, so a restart loop does not hammer
// the providers) and then every hour, until ctx ends.
func (s *Service) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}
	s.Tick(ctx)
	t := time.NewTicker(checkEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}

func (s *Service) Recent(ctx context.Context, limit int32) ([]db.RateFetch, error) {
	return s.store.ListRateFetches(ctx, limit)
}

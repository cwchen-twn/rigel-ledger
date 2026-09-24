package routes

import (
	"net/http"
	"time"

	"github.com/cwchen-twn/rigel-ledger/internal/db"
	"github.com/cwchen-twn/rigel-ledger/internal/response"
)

type RateFetchDTO struct {
	ID        int64   `json:"id"`
	Source    string  `json:"source"`
	RateDate  *string `json:"rate_date"`
	Requested *string `json:"requested"`
	Rates     int32   `json:"rates"`
	Skipped   int32   `json:"skipped"`
	Error     string  `json:"error"`
	FetchedAt string  `json:"fetched_at"`
}

func dateStr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02")
	return &s
}

func rateFetchDTO(f db.RateFetch) RateFetchDTO {
	return RateFetchDTO{ID: f.ID, Source: f.Source, RateDate: dateStr(f.RateDate), Requested: dateStr(f.Requested),
		Rates: f.Rates, Skipped: f.Skipped, Error: f.Error, FetchedAt: timestamp(f.FetchedAt)}
}

type RateStatusDTO struct {
	// False when RATES_ENABLED=false or the scheduler is not wired (tests).
	Scheduler bool           `json:"scheduler"`
	Recent    []RateFetchDTO `json:"recent"`
}

// rateFetches
//
//	@Summary	Recent exchange-rate fetches (admin)
//	@Tags		admin
//	@Produce	json
//	@Success	200	{object}	RateStatusDTO
//	@Router		/api/admin/rates [get]
func (h *handlers) rateFetches(w http.ResponseWriter, r *http.Request) {
	out := RateStatusDTO{Scheduler: h.rates != nil, Recent: []RateFetchDTO{}}
	if h.rates != nil {
		rows, err := h.rates.Recent(r.Context(), 30)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		for _, f := range rows {
			out.Recent = append(out.Recent, rateFetchDTO(f))
		}
	}
	response.JSON(w, http.StatusOK, out)
}

type refreshRatesRequest struct {
	// A past day to backfill; empty for the latest rates.
	Date *Date `json:"date" swaggertype:"string" format:"date"`
}

// refreshRates
//
//	@Summary	Fetch the latest exchange rates now, or one past day's (admin)
//	@Tags		admin
//	@Accept		json
//	@Produce	json
//	@Param		body	body		refreshRatesRequest	false	"optional day"
//	@Success	200		{object}	RateFetchDTO
//	@Failure	502		{object}	response.ErrorBody	"every provider failed"
//	@Router		/api/admin/rates/refresh [post]
func (h *handlers) refreshRates(w http.ResponseWriter, r *http.Request) {
	var req refreshRatesRequest
	if err := response.Decode(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	if h.rates == nil {
		response.Error(w, http.StatusConflict, "rates_disabled", "the exchange-rate fetch is not running", nil)
		return
	}
	res, err := h.rates.Refresh(r.Context(), req.Date.timePtr())
	if err != nil {
		h.logger.Warn("rate refresh failed", "error", err)
		response.Error(w, http.StatusBadGateway, "rates_unavailable", err.Error(), nil)
		return
	}
	h.auditAdmin(r, "admin_rates_refreshed")
	response.JSON(w, http.StatusOK, rateFetchDTO(res))
}

type CurrentRateDTO struct {
	Currency string `json:"currency"`
	// 1 unit of currency in the book's base; null when no rate is known.
	Rate *string `json:"rate"`
}

// currentRates
//
//	@Summary	Today's rate of each currency the book uses, in its base
//	@Tags		prices
//	@Produce	json
//	@Param		bookID	path	int	true	"book id"
//	@Success	200		{array}	CurrentRateDTO
//	@Router		/api/books/{bookID}/rates/current [get]
func (h *handlers) currentRates(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.CurrentRates(r.Context(), access(r), todayUTC())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	out := make([]CurrentRateDTO, len(rows))
	for i, c := range rows {
		out[i] = CurrentRateDTO{Currency: c.Currency}
		if c.Known {
			s := c.Rate.Round(8).String()
			out[i].Rate = &s
		}
	}
	response.JSON(w, http.StatusOK, out)
}

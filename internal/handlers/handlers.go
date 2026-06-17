package handlers

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/shenthark/fuel-tracker/internal/auth"
	"github.com/shenthark/fuel-tracker/internal/db"
)

type Handlers struct {
	DB      *db.DB
	AuthSvc *auth.Service
}

func New(d *db.DB, authSvc *auth.Service) *Handlers {
	return &Handlers{DB: d, AuthSvc: authSvc}
}

type EntryResponse struct {
	db.Entry
	TotalCost float64  `json:"total_cost"`
	Kmpl      *float64 `json:"kmpl,omitempty"`
}

type Stats struct {
	Month       string  `json:"month"`
	TotalKm     float64 `json:"total_km"`
	TotalCost   float64 `json:"total_cost"`
	TotalLiters float64 `json:"total_liters"`
	AvgKmpl     float64 `json:"avg_kmpl"`
	FillCount   int     `json:"fill_count"`
}

func NewRouter(h *Handlers, staticFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Post("/api/login", h.login)

	r.Route("/api", func(r chi.Router) {
		r.Use(h.AuthSvc.Middleware)
		r.Get("/entries", h.listEntries)
		r.Post("/entries", h.createEntry)
		r.Get("/entries/{id}", h.getEntry)
		r.Put("/entries/{id}", h.updateEntry)
		r.Delete("/entries/{id}", h.deleteEntry)
		r.Get("/stats", h.stats)
		r.Get("/export", h.export)
	})

	if staticFS != nil {
		fileServer := http.FileServer(http.FS(staticFS))
		r.Handle("/*", fileServer)
	} else {
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "static assets not embedded", http.StatusInternalServerError)
		})
	}

	return r
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handlers) login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password required")
		return
	}
	tok, err := h.AuthSvc.Login(r.Context(), body.Username, body.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": tok})
}

func (h *Handlers) listEntries(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	entries, err := h.DB.ListEntries(r.Context(), month)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponses(entries))
}

func (h *Handlers) createEntry(w http.ResponseWriter, r *http.Request) {
	in, err := decodeEntry(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := h.DB.CreateEntry(r.Context(), in)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	got, err := h.DB.GetEntry(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toResponse(got, nil))
}

func (h *Handlers) getEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	got, err := h.DB.GetEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	all, _ := h.DB.ListEntries(r.Context(), "")
	writeJSON(w, http.StatusOK, toResponsesWithKmpl(all, got.ID))
}

func (h *Handlers) updateEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	in, err := decodeEntry(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.DB.UpdateEntry(r.Context(), id, in); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	got, err := h.DB.GetEntry(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toResponse(got, nil))
}

func (h *Handlers) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.DB.DeleteEntry(r.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) stats(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if !isValidMonth(month) {
		writeError(w, http.StatusBadRequest, "month query param required as YYYY-MM")
		return
	}
	entries, err := h.DB.ListEntries(r.Context(), month)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(entries) == 0 {
		writeJSON(w, http.StatusOK, Stats{Month: month})
		return
	}
	asc := reverseInPlace(append([]db.Entry(nil), entries...))
	s := Stats{Month: month, FillCount: len(asc)}
	for i, e := range asc {
		s.TotalLiters += e.Liters
		s.TotalCost += e.Liters * e.PricePerL
		if i > 0 {
			diff := e.Odometer - asc[i-1].Odometer
			if diff > 0 {
				s.TotalKm += diff
			}
		}
	}
	if s.TotalLiters > 0 {
		s.AvgKmpl = s.TotalKm / s.TotalLiters
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handlers) export(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if !isValidDate(from) || !isValidDate(to) {
		writeError(w, http.StatusBadRequest, "from and to required as YYYY-MM-DD")
		return
	}
	all, err := h.DB.ListEntries(r.Context(), "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var filtered []db.Entry
	for _, e := range all {
		if e.Date >= from && e.Date <= to {
			filtered = append(filtered, e)
		}
	}
	asc := reverseInPlace(append([]db.Entry(nil), filtered...))

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="fuel-export.csv"`)
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"date", "odometer", "liters", "price_per_l", "total_cost", "fuel_type", "kmpl", "notes"}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i, e := range asc {
		var kmplStr string
		if i > 0 {
			kmpl := (e.Odometer - asc[i-1].Odometer) / e.Liters
			kmplStr = fmt.Sprintf("%.2f", kmpl)
		}
		if err := cw.Write([]string{
			e.Date,
			fmt.Sprintf("%.2f", e.Odometer),
			fmt.Sprintf("%.2f", e.Liters),
			fmt.Sprintf("%.2f", e.PricePerL),
			fmt.Sprintf("%.2f", e.Liters*e.PricePerL),
			e.FuelType,
			kmplStr,
			e.Notes,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

type entryInput struct {
	Date      string  `json:"date"`
	Odometer  float64 `json:"odometer"`
	Liters    float64 `json:"liters"`
	PricePerL float64 `json:"price_per_l"`
	FuelType  string  `json:"fuel_type"`
	Notes     string  `json:"notes"`
}

func decodeEntry(r *http.Request) (db.Entry, error) {
	var in entryInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return db.Entry{}, errors.New("invalid JSON body")
	}
	if !isValidDate(in.Date) {
		return db.Entry{}, errors.New("date required as YYYY-MM-DD")
	}
	if in.Odometer <= 0 {
		return db.Entry{}, errors.New("odometer must be > 0")
	}
	if in.Liters <= 0 {
		return db.Entry{}, errors.New("liters must be > 0")
	}
	if in.PricePerL <= 0 {
		return db.Entry{}, errors.New("price_per_l must be > 0")
	}
	if in.FuelType == "" {
		in.FuelType = "regular"
	}
	if in.FuelType != "regular" && in.FuelType != "premium" {
		return db.Entry{}, errors.New("fuel_type must be 'regular' or 'premium'")
	}
	return db.Entry{
		Date:      in.Date,
		Odometer:  in.Odometer,
		Liters:    in.Liters,
		PricePerL: in.PricePerL,
		FuelType:  in.FuelType,
		Notes:     in.Notes,
	}, nil
}

var (
	dateRE  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	monthRE = regexp.MustCompile(`^\d{4}-\d{2}$`)
)

func isValidDate(s string) bool {
	if !dateRE.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func isValidMonth(s string) bool {
	if !monthRE.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01", s)
	return err == nil
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func toResponse(e db.Entry, kmpl *float64) EntryResponse {
	return EntryResponse{
		Entry:     e,
		TotalCost: round2(e.Liters * e.PricePerL),
		Kmpl:      kmpl,
	}
}

func toResponses(entries []db.Entry) []EntryResponse {
	if len(entries) == 0 {
		return []EntryResponse{}
	}
	asc := append([]db.Entry(nil), entries...)
	reverseInPlace(asc)
	out := make([]EntryResponse, len(entries))
	for i, e := range entries {
		idx := indexOf(asc, e.ID)
		out[i] = toResponse(e, computeKmpl(asc, idx))
	}
	return out
}

func toResponsesWithKmpl(all []db.Entry, targetID int64) EntryResponse {
	asc := append([]db.Entry(nil), all...)
	reverseInPlace(asc)
	idx := indexOf(asc, targetID)
	if idx < 0 {
		return toResponse(db.Entry{}, nil)
	}
	return toResponse(asc[idx], computeKmpl(asc, idx))
}

func computeKmpl(asc []db.Entry, idx int) *float64 {
	if idx <= 0 {
		return nil
	}
	v := (asc[idx].Odometer - asc[idx-1].Odometer) / asc[idx].Liters
	return &v
}

func indexOf(arr []db.Entry, id int64) int {
	for i, e := range arr {
		if e.ID == id {
			return i
		}
	}
	return -1
}

func reverseInPlace(s []db.Entry) []db.Entry {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"koperasi/internal/middleware"
	"koperasi/internal/service"
)

type Handler struct {
	authSvc       *service.AuthService
	memberSvc     *service.MemberService
	savingSvc     *service.SavingService
	loanSvc       *service.LoanService
	paymentSvc    *service.PaymentService
	settlementSvc *service.SettlementService
	reportSvc     *service.ReportService
	cookieSecure  bool
	templates     map[string]*template.Template
}

// NewHandler membuat Handler baru dan memuat template HTML sekali saat startup.
func NewHandler(
	authSvc *service.AuthService,
	memberSvc *service.MemberService,
	savingSvc *service.SavingService,
	loanSvc *service.LoanService,
	paymentSvc *service.PaymentService,
	settlementSvc *service.SettlementService,
	reportSvc *service.ReportService,
	cookieSecure bool,
	templatesDir string,
) (*Handler, error) {
	tmplMap, err := parseTemplates(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &Handler{
		authSvc:       authSvc,
		memberSvc:     memberSvc,
		savingSvc:     savingSvc,
		loanSvc:       loanSvc,
		paymentSvc:    paymentSvc,
		settlementSvc: settlementSvc,
		reportSvc:     reportSvc,
		cookieSecure:  cookieSecure,
		templates:     tmplMap,
	}, nil
}

func parseTemplates(rootDir string) (map[string]*template.Template, error) {
	basePath := filepath.Join(rootDir, "layouts", "base.html")

	funcMap := template.FuncMap{
		"formatRupiah": formatRupiah,
		"formatDate":   formatDate,
		"formatMonth":  formatMonth,
		"add": func(a, b int64) int64 { return a + b },
		"mul": func(a, b int64) int64 { return a * b },
		"div": func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}

	pages := []string{
		"auth/login.html",
		"dashboard/index.html",
		"members/index.html",
		"members/form.html",
		"members/detail.html",
		"members/exit.html",
		"savings/index.html",
		"savings/deposit.html",
		"savings/withdrawal.html",
		"savings/receipt.html",
		"loans/index.html",
		"loans/form.html",
		"loans/detail.html",
		"loans/schedule.html",
		"payments/form.html",
		"payments/receipt.html",
		"settlements/preview.html",
		"settlements/receipt.html",
		"reports/loan_card.html",
		"reports/arrears.html",
		"reports/monthly.html",
	}

	tmpls := make(map[string]*template.Template)

	for _, page := range pages {
		pagePath := filepath.Join(rootDir, filepath.FromSlash(page))
		t, err := template.New(filepath.Base(pagePath)).Funcs(funcMap).ParseFiles(basePath, pagePath)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", page, err)
		}
		tmpls[page] = t
	}

	return tmpls, nil
}

func (h *Handler) newTemplateData(r *http.Request) map[string]any {
	data := make(map[string]any)
	sess := middleware.SessionFromContext(r.Context())
	user := middleware.UserFromContext(r.Context())
	if user != nil {
		data["User"] = user
	}
	if sess != nil {
		data["CSRFToken"] = sess.CSRFToken
	}
	return data
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	h.Render(w, r, name, data)
}

func (h *Handler) renderNotFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, status int, msg string) {
	http.Error(w, msg, status)
}

// Render mengeksekusi template dengan nama tertentu dan menyertakan konteks pengguna serta CSRF.
func (h *Handler) Render(w http.ResponseWriter, r *http.Request, name string, data map[string]any) {
	t, ok := h.templates[name]
	if !ok {
		http.Error(w, fmt.Sprintf("Template %s not found", name), http.StatusInternalServerError)
		return
	}

	if data == nil {
		data = make(map[string]any)
	}

	// Injeksi otomatis sesi, pengguna, dan CSRF token ke template data
	sess := middleware.SessionFromContext(r.Context())
	user := middleware.UserFromContext(r.Context())

	if _, exists := data["User"]; !exists && user != nil {
		data["User"] = user
	}
	if _, exists := data["CSRFToken"]; !exists && sess != nil {
		data["CSRFToken"] = sess.CSRFToken
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
		http.Error(w, fmt.Sprintf("Template execution error: %v", err), http.StatusInternalServerError)
	}
}

func formatRupiah(amount int64) string {
	if amount < 0 {
		return "-" + formatRupiah(-amount)
	}
	s := strconv.FormatInt(amount, 10)
	n := len(s)
	if n <= 3 {
		return "Rp" + s
	}
	var b strings.Builder
	b.WriteString("Rp")
	rem := n % 3
	if rem > 0 {
		b.WriteString(s[:rem])
		if rem < n {
			b.WriteString(".")
		}
	}
	for i := rem; i < n; i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < n {
			b.WriteString(".")
		}
	}
	return b.String()
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("02/01/2006")
}

func formatMonth(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("01/2006")
}

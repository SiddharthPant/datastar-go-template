package auth

import (
	"datastar-go/features/auth/pages"
	"log/slog"
	"net/http"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if err := pages.LoginPage().Render(r.Context(), w); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	slog.Info("login", "email", email, "password", password)
	http.Redirect(w, r, "/", http.StatusSeeOther)

}

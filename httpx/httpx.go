// Package httpx — адаптер rolego для net/http: middleware Require разбирает
// (subj, res) из запроса и проверяет право перед передачей следующему обработчику.
package httpx

import (
	"net/http"

	"github.com/igor-kpsv/rolego"
)

// Require — middleware ролевой авторизации для net/http: разбирает (subj, res)
// из запроса через parse и проверяет checker.Check на право perm в контексте
// r.Context(). Allow передаёт запрос следующему обработчику (ничего не пишет),
// иначе отвечает сам: 400 — ошибка parse (текст ошибки в теле), 500 — ошибка
// Check (включая sentinel-ошибки ядра вроде ErrZeroPerm), 403 — решение Deny.
func Require[S, R any](checker *rolego.Checker[S, R], perm rolego.Perm, parse func(*http.Request) (S, R, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subj, res, err := parse(r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			dec, err := checker.Check(r.Context(), subj, res, perm)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if !dec.Allow() {
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

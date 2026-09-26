package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/igor-kpsv/rolego"
)

// Сценарий тестов «ключ — дверь»: роль держателя ключа открывает дверь.
const (
	kindDoor rolego.Kind = 200

	roleKeyholder rolego.Role = 1 << 10

	openDoor rolego.Perm = 1 << 5
)

// doorResource — ресурс теста: звенья двери.
type doorResource struct {
	scopes []rolego.Scope
}

// holderResolver — резолвер «держатель ключа из карты»: роль даёт только на звене
// двери и только субъектам из карты.
type holderResolver map[string]rolego.Role

func (r holderResolver) RolesAt(_ context.Context, subj string, link rolego.Link) (rolego.Roles, error) {
	if link.Kind != kindDoor {
		return rolego.RolesOf(0), nil
	}
	return rolego.RolesOf(r[subj]), nil
}

// failingResolver — резолвер со сбоем: любая проверка возвращает ошибку (модель
// падения хранилища ролей).
type failingResolver struct{ err error }

func (r failingResolver) RolesAt(context.Context, string, rolego.Link) (rolego.Roles, error) {
	return rolego.RolesOf(0), r.err
}

// ctxMarker — тип ключа контекста для проверки передачи r.Context() в Check.
type ctxMarker struct{}

// ctxResolver — резолвер, дающий роль только если в контексте запроса есть
// значение ключа ctxMarker: без него субъект прав не имеет.
type ctxResolver struct{}

func (ctxResolver) RolesAt(ctx context.Context, _ string, link rolego.Link) (rolego.Roles, error) {
	if link.Kind != kindDoor {
		return rolego.RolesOf(0), nil
	}
	if ctx.Value(ctxMarker{}) != nil {
		return rolego.RolesOf(roleKeyholder), nil
	}
	return rolego.RolesOf(0), nil
}

// Компиляционные проверки: все резолверы реализуют rolego.Resolver.
var (
	_ rolego.Resolver[string, doorResource] = holderResolver{}
	_ rolego.Resolver[string, doorResource] = failingResolver{}
	_ rolego.Resolver[string, doorResource] = ctxResolver{}
)

// storeErr — модельная ошибка сбоя проверки.
var storeErr = errors.New("roles store is down")

// newChecker собирает Checker сценария «ключ — дверь» с заданным резолвером.
func newChecker(res rolego.Resolver[string, doorResource]) *rolego.Checker[string, doorResource] {
	c, err := rolego.New[string, doorResource](
		rolego.Type[string, doorResource](kindDoor),
		rolego.MapScopes[string, doorResource](func(d doorResource) []rolego.Scope { return d.scopes }),
		rolego.Resolve[string, doorResource](res),
		rolego.WithPolicy[string, doorResource](
			rolego.Matrices{kindDoor: {roleKeyholder: openDoor}},
			rolego.NewScopeChain(rolego.Level(kindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}
	return c
}

// parseSubj возвращает parse «субъект из параметра, ресурс — дверь id=7».
func parseSubj(subj string) func(*http.Request) (string, doorResource, error) {
	return func(*http.Request) (string, doorResource, error) {
		return subj, doorResource{scopes: []rolego.Scope{{Kind: kindDoor, ID: 7}}}, nil
	}
}

func TestRequire(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("opened"))
	})

	keyCard := newChecker(holderResolver{"alice": roleKeyholder})
	broken := newChecker(failingResolver{err: storeErr})

	parseErrText := "no subject"
	parseErr := func(*http.Request) (string, doorResource, error) {
		return "", doorResource{}, errors.New(parseErrText)
	}

	for _, tt := range []struct {
		name       string
		checker    *rolego.Checker[string, doorResource]
		parse      func(*http.Request) (string, doorResource, error)
		wantStatus int
		wantBody   string
	}{
		{"Allow — next вызван, ответ 200 не перезаписан", keyCard, parseSubj("alice"), http.StatusOK, "opened"},
		{"Deny — 403", keyCard, parseSubj("bob"), http.StatusForbidden, "Forbidden\n"},
		{"ошибка parse — 400, текст ошибки в теле", keyCard, parseErr, http.StatusBadRequest, parseErrText + "\n"},
		{"ошибка Check — 500", broken, parseSubj("alice"), http.StatusInternalServerError, "Internal Server Error\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := Require(tt.checker, openDoor, tt.parse)(next)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/door/7", nil)
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestRequireContext(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := Require(newChecker(ctxResolver{}), openDoor, parseSubj("alice"))(next)

	// Без значения в контексте запроса Check совпадающего ctx не видит — Deny.
	req := httptest.NewRequest(http.MethodGet, "/door/7", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("без ctx status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	// Со значением в контексте запроса роли есть — Allow, запрос доходит до next.
	req = httptest.NewRequest(http.MethodGet, "/door/7", nil)
	req = req.WithContext(context.WithValue(req.Context(), ctxMarker{}, "ok"))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("с ctx status = %d, want %d", rec.Code, http.StatusOK)
	}
}

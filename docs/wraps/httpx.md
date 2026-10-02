# HTTP-мидлвара (httpx)

Готовый мост из ролевой проверки в `net/http`: мидлвара сама достаёт `(subj, res)` из запроса, вызывает `Check` и отвечает статусом — тебе остаётся описать, как прочитать субъекта и ресурс из запроса.

## Сигнатура

```go
func Require[S, R any](checker *rolego.Checker[S, R], perm rolego.Perm,
	parse func(*http.Request) (S, R, error)) func(http.Handler) http.Handler
```

Три аргумента:

- `checker` — собранный `rolego.Checker`;
- `perm` — право, которое проверяется на каждом запросе;
- `parse` — **твой код**: извлекает из запроса субъекта и ресурс; ошибка `parse` — это «плохой запрос» (400).

Результат — обычная `func(http.Handler) http.Handler`, поэтому `Require` встраивается в любую цепочку мидлвар.

## Что делает для каждого запроса

1. `parse(r)` — достаёт `(subj, res)` из запроса.
2. Ошибка `parse` → **400**, текст ошибки в теле.
3. `checker.Check(r.Context(), subj, res, perm)`.
4. Ошибка `Check` (включая `rolego.ErrZeroPerm` и сбой резолвера) → **500**.
5. Решение `Deny` → **403**.
6. `Allow` → передаёт запрос следующему обработчику, ничего не пишет.

Для `parse` доступен только URL, заголовки и тело — можно завести субъекта из заголовка, а ресурс — из пути.

## Один полный пример

Документ №42; писать может только редактор. Субъект читаем из заголовка `X-User`, ресурс фиксирован — документ 42.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/httpx"
)

const (
	RoleViewer rolego.Role = 1 << iota
	RoleEditor
)

const (
	PermRead  rolego.Perm = 1 << iota
	PermWrite
)

const KindDocument rolego.Kind = 1

type Document struct{ ID uint64 }

func scopes(d Document) []rolego.Scope {
	return []rolego.Scope{{Kind: KindDocument, ID: d.ID}}
}

type members map[string]rolego.Role

func (m members) RolesAt(_ context.Context, subj string, _ Document, _ rolego.Link) (rolego.Resolved, error) {
	return rolego.Resolved{Roles: rolego.RolesOf(m[subj])}, nil
}

func main() {
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](members{"alice": RoleEditor}),
		rolego.WithPolicy[string, Document](
			rolego.Matrices{KindDocument: {
				RoleViewer: PermRead,
				RoleEditor: PermRead | PermWrite,
			}},
			rolego.NewScopeChain(rolego.Level(KindDocument)),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	// parse: субъект из заголовка X-User.
	parse := func(r *http.Request) (string, Document, error) {
		user := r.Header.Get("X-User")
		if user == "" {
			return "", Document{}, errors.New("пустой субъект")
		}
		return user, Document{ID: 42}, nil
	}

	handler := httpx.Require(checker, PermWrite, parse)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, user := range []string{"alice", "bob", ""} {
		req := httptest.NewRequest(http.MethodGet, "/documents/42", nil)
		req.Header.Set("X-User", user)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		fmt.Printf("%q — %d\n", user, rec.Code)
	}

	// Output:
	// "alice" — 200
	// "bob" — 403
	// "" — 400
}
```

## Контекст запроса

В `Check` уходит именно `r.Context()`. Если мидлвара выше положила в контекст метаданные (trace-id, время, субъекта), их увидит твой резолвер и аудит-логгеры.

## Композиция с другими мидлварами

`Require` возвращает стандартный `func(http.Handler) http.Handler` — встраивается в `http.ServeMux`, `chi`, `gorilla` и собственные цепочки. Полезная связка: снаружи [аудит](audit.md), чтобы каждое решение попадало в лог, внутри — `Require`.

Если твой веб-фреймворк — Chi, Gin, Echo или Fiber, мосты на каждый из них собраны на странице [Подключение к фреймворкам](frameworks.md).
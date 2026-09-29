# Проверка права (Check)

Главный сценарий библиотеки: «может ли субъект сделать действие над ресурсом?». После того как `Checker` собран ([Быстрый старт](quickstart.md)), ты только задаёшь вопросы.

## Сигнатура

```go
func (c *Checker[S, R]) Check(ctx context.Context, subj S, res R, perm Perm) (Decision, error)
```

- `ctx` — контекст вызова: доходит до твоего резолвера, в него удобно прокинуть trace-id, дедлайны;
- `subj` — субъект: пользователь, сервис, сессия — тот, кто совершает действие;
- `res` — ресурс: над чем совершается действие;
- `perm` — право: что именно проверяется.

Ответ — пара `(Decision, error)`. Решение бывает двух значений — `rolego.Allow` и `rolego.Deny`:

```go
dec.Allow() // bool: true — доступ разрешён, false — нет
```

## Почему не один bool

`Allow` и «ошибка» — разные вещи, и путать их опасно:

1. **Ошибка резолвера — не Deny.** Упала база членства? Ответить «запрещено всем» — тихо заблокировать продукты; «разрешено всем» — дыра. Обе беды хуже, чем явная ошибка: её видно в логах, она превращается в 500-й статус.
2. **Удобная bool-оболочка есть отдельно** — `Allows` для тех мест, где причина отказа не важна.

## Классический разбор ответа

```go
decision, err := checker.Check(ctx, user, doc, PermWrite)
if err != nil {
	// 500 + аудит: резолвер недоступен — ответить «запрещено» нельзя.
	return 500
}
if !decision.Allow() {
	// Нет права — 403.
	return 403
}
```

## bool-обёртка `Allows`

Когда важен только сам факт, а ошибку обрабатывать негде:

```go
if checker.Allows(ctx, user, doc, PermWrite) {
	// разрешено (ошибка трактуется как отказ)
}
```

!!! warning "Семантика"
    `Allows` при любой ошибке — включая `rolego.ErrZeroPerm` — возвращает `false`. Если нужно отличить «нет права» от «сломались данные», используй `Check`.

## Нулевое право

`perm == 0` — бессмысленный запрос: проверять нечего. Ядро отвечает `(Deny, rolego.ErrZeroPerm)`. Права объявляй через `1 << iota`, чтобы нулевой бит не достался реальному действию.

## Правило «пусто → Deny»

Несколько штатных ситуаций дают `Deny` **без ошибки** — это не сбой, а политика «нет данных — доступа нет»:

- у ресурса нет звеньев (экстрактор вернул пустой список);
- после комбинации масок по оси итог пуст — субъект не имеет ни одной роли ни на одном звене;
- в матрице нет роли субъекта (роль есть, но её записи в матрице нет — право не выписано).

## Один полный пример

Сюжет: документ; Алиса — редактор, Боб — зритель. Смотрим три случая: обычный отказ, технический сбой резолвера и нулевое право.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/igor-kpsv/rolego"
)

const (
	RoleViewer  rolego.Role = 1 << iota
	RoleEditor
)

const (
	PermRead  rolego.Perm = 1 << iota
	PermWrite
)

const KindDocument rolego.Kind = 1

type Document struct {
	ID uint64
}

func scopes(d Document) []rolego.Scope {
	return []rolego.Scope{{Kind: KindDocument, ID: d.ID}}
}

// failResolver — резолвер, который умеет «падать», чтобы показать разницу
// между отказом по политике и ошибкой по техническим причинам.
type failResolver struct {
	fail bool
}

func (r failResolver) RolesAt(_ context.Context, subj string, _ rolego.Link) (rolego.Roles, error) {
	if r.fail {
		return rolego.RolesOf(0), errors.New("хранилище ролей недоступно")
	}
	if subj == "alice" {
		return rolego.RolesOf(RoleEditor), nil
	}
	return rolego.RolesOf(RoleViewer), nil
}

func main() {
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](failResolver{}),
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
	doc := Document{ID: 42}

	dec, err := checker.Check(context.Background(), "alice", doc, PermWrite)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("alice write:", dec.Allow()) // true

	dec, err = checker.Check(context.Background(), "bob", doc, PermWrite)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("bob write:", dec.Allow()) // false

	fmt.Println("bob read:", checker.Allows(context.Background(), "bob", doc, PermRead)) // true

	failing, _ := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](failResolver{fail: true}),
		rolego.WithPolicy[string, Document](
			rolego.Matrices{KindDocument: {RoleEditor: PermWrite}},
			rolego.NewScopeChain(rolego.Level(KindDocument)),
		),
	)
	_, err = failing.Check(context.Background(), "alice", doc, PermWrite)
	fmt.Println("resolver error:", err != nil) // true

	_, err = checker.Check(context.Background(), "alice", doc, 0)
	fmt.Println("zero perm:", errors.Is(err, rolego.ErrZeroPerm)) // true

	// Output:
	// alice write: true
	// bob write: false
	// bob read: true
	// resolver error: true
	// zero perm: true
}
```

## Когда `Deny`, а когда ошибка

| Ситуация | Что возвращается |
|---|---|
| `perm == 0` | `(Deny, rolego.ErrZeroPerm)` |
| у ресурса нет звеньев | `(Deny, nil)` |
| итоговая маска ролей пуста | `(Deny, nil)` |
| упал резолвер | `(Deny, err)` — ошибка резолвера |
| право не выписано в матрице | `(Deny, nil)` |

Дальше: обратный вопрос «кто из списка может действие?» — [Кто имеет право (WhoCan)](who-can.md).
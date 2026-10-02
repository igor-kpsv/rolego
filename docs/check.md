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

## Составное право

`perm` не обязан быть одним битом. `Check(ctx, user, doc, PermRead|PermWrite)` спрашивает «есть ли хоть одно из этих прав?» — ядро смотрит пересечение прав ролей субъекта с запрошенной маской: **`Allow`, когда покрыт хотя бы один бит, а не все**. Объединение битов в `perm` работает как OR, а не AND:

```go
// true, если у субъекта есть ИЛИ чтение, ИЛИ запись — матрица покрыла хоть один бит
decision, err := checker.Check(ctx, user, doc, PermRead|PermWrite)
```

Если нужен именно конъюнктивный смысл («читать и писать одновременно»), проверяй биты по отдельности двумя `Check`.

## Персональные права (гранты)

Матрица статична: «роль → права». Но у субъекта бывают **персональные** права поверх системной роли — таблица в БД, редактируемая через API. Синтетическая роль под каждую комбинацию грантов нежизнеспособна: комбинаций много, всё меняется динамически.

Rolego решает это отдельным каналом прав: резолвер возвращает `Resolved{Grants}` — маску `Perm`, которая добавляется к правам ролей по OR:

```go
type Resolved struct {
	Roles  Roles
	Grants Perm // персональные права субъекта на звене; ноль — грантов нет
}

func (r personalRights) RolesAt(_ context.Context, subj string, res Document, _ rolego.Link) (rolego.Resolved, error) {
	return rolego.Resolved{
		Roles:  rolego.RolesOf(r.base[subj]), // системная роль
		Grants: r.extra[subj],                // персональные права из БД
	}, nil
}
```

Грант — «что тебе можно дополнительно», а не «кто ты»: он не требует роли, не выписывается в матрице и не порождает запись в `RolesFor`. Правила:

- **Гранты OR-ятся по всем звеньям оси**, независимо от правила комбинации ролей (`Nearest`/`Union`): персональные права — аддитивная надбавка, а не «роль, которую нужно найти на самом глубоком звене».
- Итоговое решение: `(права ролей | гранты) & perm != 0` — как и в `Matrix.Allow`, «хотя бы один бит».
- **Грант без роли работает**: резолвер вернул только `Grants` — `Check` по этому праву даёт `Allow`.

**Профит:** системная роль и пер-персональные права живут раздельно — матрица не разрастается под комбинации грантов, и ничего не мутируется на запрос.

## Правило «пусто → Deny»

Несколько штатных ситуаций дают `Deny` **без ошибки** — это не сбой, а политика «нет данных — доступа нет»:

- у ресурса нет звеньев (экстрактор вернул пустой список);
- после комбинации масок по оси итог пуст — субъект не имеет ни одной роли ни на одном звене;
- в матрице нет роли субъекта (роль есть, но её записи в матрице нет — право не выписано).

## Ролевой запрос без права: `RolesFor`

`Check` отвечает на вопрос «можно ли действие?». Не менее частый вопрос — «какие роли у субъекта на этом ресурсе?»: владелец ресурса, менеджер скоупа, дежурный. До появления `RolesFor` его выражали простановочным битом в матрице — громоздко и размывает смысл матрицы.

```go
func (c *Checker[S, R]) RolesFor(ctx context.Context, subj S, res R) (Roles, error)
```

`RolesFor` — тот же конвейер, что внутри `Check` (`MapScopes` → `RolesAt` по звеньям → объединение масок по правилу оси), но **без сверки с матрицей**. Результат — маска ролей, которую проверяешь методами `Roles`:

```go
roles, err := checker.RolesFor(ctx, user, doc)
if err != nil {
	return err // упал резолвер — отвечать по догадке нельзя
}
if roles.Has(RoleOwner) {
	// владелец: показать «настройки» и другие владельческие действия
}
```

Правила те же, что в `Check`:

| Ситуация | Что возвращает `RolesFor` |
|---|---|
| упал резолвер | `(RolesOf(0), err)` — ошибка резолвера |
| у ресурса нет звеньев | `(RolesOf(0), nil)` |
| маски ролей пусты | `(RolesOf(0), nil)` |

!!! note "Иерархия в ролях не раскрывается"
    Если `Checker` собран с [`WithHierarchy`](roles.md), `RolesFor` всё равно вернёт сырые роли резолвера: иерархия добавила права предков в матрицы при сборке, но в маску ролей не подмешивается. «Эффективные» роли (предки включительно) вычисляй сам по своей иерархии.

**Профит:** вопросы «кто владелец», «сколько у субъекта ролей на скоупе», «есть ли дежурный с этой ролью» — без простановочных битов в матрице. Проверку конкретного звена (например, только самого ресурса) делаешь напрямую через свой `Resolver.RolesAt` — `RolesFor` даёт результат, объединённый по всей оси.

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

func (r failResolver) RolesAt(_ context.Context, subj string, _ Document, _ rolego.Link) (rolego.Resolved, error) {
	if r.fail {
		return rolego.Resolved{}, errors.New("хранилище ролей недоступно")
	}
	if subj == "alice" {
		return rolego.Resolved{Roles: rolego.RolesOf(RoleEditor)}, nil
	}
	return rolego.Resolved{Roles: rolego.RolesOf(RoleViewer)}, nil
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

| Ситуация | Что возвращается | Причина (`CheckResult`) |
|---|---|---|
| `perm == 0` | `(Deny, rolego.ErrZeroPerm)` | `DenyReasonNoPerm` |
| у ресурса нет звеньев | `(Deny, nil)` | `DenyReasonNoRoles` |
| итоговая маска ролей пуста, грантов нет | `(Deny, nil)` | `DenyReasonNoRoles` |
| упал резолвер | `(Deny, err)` — ошибка резолвера | незначима (сбой, а не политика) |
| право не выписано (роли или гранты есть) | `(Deny, nil)` | `DenyReasonNoPerm` |

## Причины отказа: `CheckResult`

`Check` возвращает `Deny` без различения причин. Для разных ответов API — «403 нет доступа к ресурсу» против «403 недостаточно прав» — есть `CheckResult`: тот же `Check`, но с причиной:

```go
type DenyReason uint8

const (
	DenyReasonNoRoles DenyReason = iota // ролей и грантов нет — «нет доступа к ресурсу»
	DenyReasonNoPerm                    // роли/гранты есть, право не выписано — «недостаточно прав»
)

type Result struct {
	Decision Decision
	Reason   DenyReason // смысл имеет только при Deny
}

func (c *Checker[S, R]) CheckResult(ctx context.Context, subj S, res R, perm Perm) (Result, error)
```

```go
r, err := checker.CheckResult(ctx, user, doc, PermWrite)
if err != nil {
	return 500 // сбой резолвера — решение не выносилось
}
switch {
case r.Decision.Allow():
	return 200
case r.Reason == rolego.DenyReasonNoRoles:
	return 403 // «нет доступа к ресурсу»
default:
	return 403 // «недостаточно прав»
}
```

`Check` — тонкая обёртка над `CheckResult`, поэтому политика не дублируется: `deny`-причины различимы только через `CheckResult`, остальные вызовы не меняются.

Дальше: обратный вопрос «кто из списка может действие?» — [Кто имеет право (WhoCan)](who-can.md).
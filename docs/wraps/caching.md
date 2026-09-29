# Кэширование (caching)

Обёртка над `Checker`, которая запоминает решения `Check`, чтобы не спрашивать резолвер (и его базу) на каждый запрос. Отдельный жизненный вопрос — **свежесть**: кэш не знает, когда роли в базе изменились. Рычаги инвалидации ты настраиваешь сам; ниже весь «флоу свежести».

## Сигнатура

```go
func Wrap[S comparable, R comparable](checker *rolego.Checker[S, R], opts ...Option) *Checker[S, R]

func WithMaxEntries(n int) Option              // лимит записей; FIFO-эвикция
func WithTTL(d time.Duration) Option           // срок жизни записи; d <= 0 — без срока
func WithVersion(version func() uint64) Option // версия данных; смена — автоматическая инвалидация
```

```go
func (c *Checker[S, R]) Invalidate(subj S, res R, perm rolego.Perm) // отменить одно решение
func (c *Checker[S, R]) InvalidateSubj(subj S)                      // отменить все решения субъекта
func (c *Checker[S, R]) Clear()                                     // сбросить кэш целиком
func (c *Checker[S, R]) Len() int                                   // число живых записей
```

`S` и `R` обязаны быть **сравнимыми** (`comparable`): ключ кэша строится на самом ресурсе `(subj, res, perm)`, потому что ядро не отдаёт наружу звенья ресурса.

## Как работает

```go
cached := caching.Wrap(checker, caching.WithMaxEntries(1000))
decision, err := cached.Check(ctx, user, doc, PermWrite) // второй раз тот же ключ — из кэша
```

- **Ключ** — `(subj, res, perm)`. Только успешные решения (и `Allow`, и `Deny`) попадают в кэш.
- **Ошибки не кэшируются.** Упавший однажды резолвер не «замораживается»: следующий `Check` снова попробует сходить в БД.
- **Потокобезопасно** — можно звать из параллельных запросов.
- `WhoCan` и `Validate` делегируются подлинному `Checker` без кэша.

## Флоу свежести: три рычага

Кэш живёт, пока данные резолвера не поменялись. Кто-то переклеил роли в базе — старые решения устарели, а обёртка этого не знает. Выбор рычага — вопрос «как сообщить кэшу, что данные изменились?».

### 1. Время: `WithTTL` — «просто, но вслепую»

Запись живёт `d` с момента создания. Время не знает, когда роли реально поменялись, и держит устаревшее решение до таймера.

**Профит:** роли меняются редко — можно позволить короткое окно устаревания.

```go
cached := caching.Wrap(checker, caching.WithTTL(time.Minute))
```

### 2. Версия данных: `WithVersion` — «данные изменились»

Автоматическая инвалидация по сигналу «данные изменились». Ты даёшь функцию `version` — что бы она ни вернула, запись валидна, только пока значение не поменялось. Решение кэшируется вместе с версией, на каждом `Check` функция вызывается снова (и на промах, и на хит), поэтому она должна быть дёшевой — чтение атомарного счётчика или 64-битный хеш состояния.

```go
var dataVersion atomic.Uint64 // счётчик изменений ролей

// в коде, обновляющем роли в БД (любая точка записи):
dataVersion.Add(1)

// в точке инициализации:
cached := caching.Wrap(checker, caching.WithVersion(dataVersion.Load))
```

Единственное требование к версии — **согласованность**: одинаковое значение обязано означать одинаковые данные. Счётчик — самый простой и надёжный способ его обеспечить; хеш снимка ролей (например, `hash/fnv`) сойдёт, но коллизии дают несвежие решения.

**Профит:** согласованность без ручных вызовов: где бы ни изменились данные — твой код, миграция, другой сервис — инкремент счётчика автоматически устаревает кэш.

### 3. Точечная отмена: `Invalidate` и `InvalidateSubj` — «знаю, что именно изменилось»

Ручная инвалидация из кода, который знает конкретику:

```go
cached.Invalidate("alice", doc, PermWrite) // одно решение: роли Алисы на этом документе пересчитаны
cached.InvalidateSubj("alice")             // все решения Алисы: её роли изменились глобально
```

Отсутствующие записи — no-op: можно звать смело.

**Профит:** точность без простоя кэша — остальные записи не трогаются. Подходит, когда точки изменения малочисленны и под твоим контролем.

## Один полный пример: версия + точечная отмена

Смотрим весь флоу свежести на счётчике вызовов резолвера: повторные `Check` не трогают резолвер, смена версии и `InvalidateSubj` заставляют пересчитать.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/caching"
)

const (
	RoleViewer rolego.Role = 1 << iota
	RoleEditor
)

const PermWrite rolego.Perm = 1

const KindDocument rolego.Kind = 1

type Document struct{ ID uint64 }

func scopes(d Document) []rolego.Scope {
	return []rolego.Scope{{Kind: KindDocument, ID: d.ID}}
}

// countingResolver считает обращения к «базе ролей».
type countingResolver struct {
	calls int
}

func (r *countingResolver) RolesAt(_ context.Context, _ string, _ rolego.Link) (rolego.Roles, error) {
	r.calls++
	return rolego.RolesOf(RoleEditor), nil
}

var dataVersion atomic.Uint64

func main() {
	rr := &countingResolver{}
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](rr),
		rolego.WithPolicy[string, Document](
			rolego.Matrices{KindDocument: {RoleEditor: PermWrite}},
			rolego.NewScopeChain(rolego.Level(KindDocument)),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	cached := caching.Wrap(checker,
		caching.WithMaxEntries(100),
		caching.WithVersion(dataVersion.Load),
	)

	doc := Document{ID: 42}
	for i := 0; i < 3; i++ {
		_, _ = cached.Check(context.Background(), "alice", doc, PermWrite)
	}
	fmt.Println("calls after 3 checks:", rr.calls) // 1 — первая проверка, дальше хит

	dataVersion.Add(1) // роли в БД изменились
	_, _ = cached.Check(context.Background(), "alice", doc, PermWrite)
	fmt.Println("calls after version bump:", rr.calls) // 2 — промах, пересчёт

	cached.InvalidateSubj("alice") // роли Алисы пересчитаны
	_, _ = cached.Check(context.Background(), "alice", doc, PermWrite)
	fmt.Println("calls after invalidate:", rr.calls) // 3 — промах, пересчёт

	fmt.Println("entries:", cached.Len()) // 1 — решение снова в кэше

	// Output:
	// calls after 3 checks: 1
	// calls after version bump: 2
	// calls after invalidate: 3
	// entries: 1
}
```

## Управление

```go
cached.Clear()     // полная инвалидация: сбросить всё
n := cached.Len()  // текущее число живых записей (истекшие при этом удаляются)
```

## Какой рычаг когда брать

| Ситуация | Рычаг |
|---|---|
| неизвестно, когда меняются данные | `WithTTL` |
| изменения глобальные и внешние (другой сервис, миграция) | `WithVersion` |
| изменения точечные и известные (пересчитаны роли одного субъекта) | `Invalidate`/`InvalidateSubj` |

Рычаги комбинируются: `WithVersion` от частых обновлений + `WithTTL` как страховочное окно.

Слабое место, которое не закрыть: кэш не замечает изменений, о которых ему не сообщили. Меняются данные где-то вне твоего кода, а ни версия, ни `Invalidate*`, ни TTL это не отражают — решение будет устаревшим. Правила инвалидации — часть твоего кода, не библиотеки.

Дальше: [Аудит (audit)](audit.md) — логирование каждого решения.
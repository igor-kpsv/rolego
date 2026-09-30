# rolego

[![MIT license](https://img.shields.io/badge/license-MIT-brightgreen.svg)](https://opensource.org/licenses/MIT)
[![test status](https://github.com/igor-kpsv/rolego/actions/workflows/tests.yml/badge.svg)](https://github.com/igor-kpsv/rolego/actions)
[![lint](https://github.com/igor-kpsv/rolego/actions/workflows/lint.yml/badge.svg)](https://github.com/igor-kpsv/rolego/actions)

Встраиваемая ролевая авторизация для Go: права — битовые маски-константы твоего пакета, откуда они берутся — решает твой код поверх твоих данных. Библиотека ничего не хранит и не знает о фреймворке.

Ядро использует только стандартную библиотеку.

## Возможности

- **Битовые маски, а не строки.** Роли и права — `1 << iota`-константы: опечатка — ошибка компиляции, пересечение и объединение — битовые операции.
- **Мы ничего не храним.** Роли субъекта вычисляет приложение из своих данных (membership, ownership, правила); в библиотеке нет ни таблиц, ни миграций.
- **Скоупы без доменных имён.** Ось уровней (`workspace → project → issue` или `tenant → team`) и правило комбинации (`Nearest`/`Union`) объявляешь ты. Ядро знает только числа.
- **Валидация на старте.** `Validate()` обходит политику и падает на противоречиях до того, как они ужаснут живого пользователя.
- **Обёртки для продакшена** — `httpx` (middleware), `caching`, `audit`: в отдельных подпакетах, ядро их не знает.

## Быстрый старт

```go
// Роли, права и типы ресурсов — константы твоего пакета.
const (
	RoleAdmin  rolego.Role = 1 << iota
	RoleEditor
)

const (
	Read  rolego.Perm = 1 << iota
	Write
)

const KindDocument rolego.Kind = 1

// Матрица «роль → права» — своя на каждый тип ресурса; бит права одного типа
// не переносится на другой.
var perms = rolego.Matrices{
	KindDocument: {
		RoleAdmin:  Read | Write,
		RoleEditor: Read,
	},
}

// Ось цепочки: один уровень — сам документ. Правило по умолчанию — Nearest.
chain := rolego.NewScopeChain(rolego.Level(KindDocument))

// Checker собирается из твоих частей: тип ресурса, экстрактор звеньев,
// резолвер ролей и политика.
checker, err := rolego.New[string, Document](
	rolego.Type[string, Document](KindDocument),
	rolego.MapScopes[string, Document](func(d Document) []rolego.Scope {
		return []rolego.Scope{{Kind: KindDocument, ID: d.ID}}
	}),
	rolego.Resolve[string, Document](myResolver), // RolesAt(ctx, subj, link) (rolego.Roles, error)
	rolego.WithPolicy[string, Document](perms, chain),
)
if err != nil {
	return err
}

// Противоречия политики проверяются на старте, а не под живым пользователем.
if err := checker.Validate(); err != nil {
	return err
}

ctx := context.Background()
doc := Document{ID: 42}

decision, err := checker.Check(ctx, "alice", doc, Write) // может ли alice писать в doc?
if err != nil {
	return err
}
if !decision.Allow() {
	// нет права — 403
}
```

## Механики

- **Роли и права** — `Role`, `Perm` (оба `uint64`), `All`, `RolesOf`; `Roles` — маска с `Has`/`HasAny`/`HasAll`/`Add`/`Remove`.
- **Типы ресурсов** — `Kind`, `Scope{Kind, ID}`, `Link{Scope, Kind, Depth}` (что резолвер получает на каждом звене).
- **Матрицы** — `Matrix map[Role]Perm`, `Matrices map[Kind]Matrix`; оценка через `Perms`/`Allow`.
- **Цепочка** — `ScopeChain` через `NewScopeChain(Level(kind), Combine(rule))`, правила `Nearest`/`Union`.
- **Комбинаторы политик** — `AllOf`, `Any`, `Except` (deny-wins), `Predicate`, `Single`.
- **Checker[S, R]** — `Type`/`MapScopes`/`Resolve`/`WithPolicy` (опции `New`), методы `Check`, `WhoCan` (обратный выбор: кто из кандидатов имеет право), `Validate`, `Allows` (bool-обёртка: ошибка трактуется как Deny).
- **Ошибки** — sentinel-значения (`ErrZeroPerm`, `ErrNilResolver`, `ErrEmptyChain`, …); сверяются через `errors.Is`, несколько противоречий `Validate` объединяет через `errors.Join`.

## Подпакеты

### httpx — middleware для net/http

```go
handler := httpx.Require(checker, OpenDoor, parse)(next)
// parse: func(*http.Request) (subj S, res R, error)
// 400 — ошибка parse, 403 — Deny, 500 — ошибка проверки
```

### caching — кэш решений

```go
cached := caching.Wrap(checker, caching.WithMaxEntries(1000), caching.WithTTL(time.Minute))
// Ключ (subj, res, perm); ошибочные проверки не кэшируются.
// Свежесть — тремя рычагами: WithTTL (время), WithVersion (версия данных —
// авто-инвалидация при смене), Invalidate/InvalidateSubj (точечная отмена).
```

### audit — логирование решений

```go
audited := audit.Log(checker, logger) // logger реализует audit.Logger{ Log(ctx, audit.Entry) }
// Логируются все исходы Check, включая ошибки; результат не меняется.
```

## Лицензия

MIT — см. [LICENSE](LICENSE).
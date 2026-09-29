# Комбинирование политик

Матрица ролей даёт «битовый» доступ: роль → набор прав. Иногда нужно больше: «писать может владелец или один из соавторов, кроме заблокированных», «только для активных аккаунтов». Такие правила из булевых условий собираются комбинаторами-политиками.

## Интерфейс `Policy`

```go
type Policy[S, R any] interface {
	Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error)
}
```

Сигнатура та же идея, что у `Checker.Check`: субъект, право, ресурс — на выходе `(Decision, error)`. Политика — это просто «функция решения» без обязательства иметь роли и резолвер.

## Простые предикаты: `Single` и `Predicate`

Оба превращают булев предикат в политику: `true` → `Allow`, `false` → `Deny`.

```go
// Predicate — условие внутри комбинаторов.
collaborator := rolego.Predicate(func(_ context.Context, subj string, _ rolego.Perm, d Document) bool {
	return slices.Contains(d.Access, subj)
})

// Single — самостоятельная процедурная политика без ролей.
importService := rolego.Single(func(_ context.Context, subj string, _ rolego.Perm, _ Document) bool {
	return subj == "import-service"
})
```

`Single` и `Predicate` работают одинаково; имя выбирает читаемость кода: `Single` — полноценная политика, `Predicate` — условие внутри большего правила.

## Комбинаторы: `AllOf`, `Any`, `Except`

```go
// Разрешить, если разрешили ВСЕ (пустой список — истинно).
edit := rolego.AllOf(owner, active)

// Разрешить, если разрешил ХОТЯ БЫ ОДИН (пустой список — ложно).
anyAccess := rolego.Any(owner, collaborator)

// base разрешает, но любое Allow из denied сводит его на нет (deny-wins).
final := rolego.Except(anyAccess, blocked)
```

Особенности комбинаторов:

- **`AllOf`** останавливается на первой ошибке или первом `Deny` (короткое замыкание).
- **`Any`** останавливается на первой ошибке или первом `Allow`.
- **`Except`** сначала спрашивает базу: если база `Deny`, исключения даже не проверяются (базовый `Allow` — обязательное условие). Если база `Allow` и любая из `denied` тоже `Allow` — итог `Deny`: исключение побеждает.

## Один полный пример

Правило доступа к документу: «владелец или соавтор, кроме заблокированных; редактирует только владелец активного документа».

```go
package main

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/igor-kpsv/rolego"
)

const PermWrite rolego.Perm = 1

type Document struct {
	OwnerID string
	Access  []string // те, кому открыт доступ
	Blocked []string // исключённые
	Active  bool     // документ не в архиве
}

func main() {
	doc := Document{
		OwnerID: "alice",
		Access:  []string{"alice", "bob"},
		Blocked: []string{"bob"},
		Active:  true,
	}

	// Предикаты — условия.
	owner := rolego.Predicate(func(_ context.Context, subj string, _ rolego.Perm, d Document) bool {
		return subj == d.OwnerID
	})
	collaborator := rolego.Predicate(func(_ context.Context, subj string, _ rolego.Perm, d Document) bool {
		return slices.Contains(d.Access, subj)
	})
	active := rolego.Predicate(func(_ context.Context, _ string, _ rolego.Perm, d Document) bool {
		return d.Active
	})
	blocked := rolego.Predicate(func(_ context.Context, subj string, _ rolego.Perm, d Document) bool {
		return slices.Contains(d.Blocked, subj)
	})

	// Комбинаторы — одно правило из условий.
	anyAccess := rolego.Any(owner, collaborator) // владелец ИЛИ соавтор
	final := rolego.Except(anyAccess, blocked)   // и не исключённый (deny-wins)
	edit := rolego.AllOf(owner, active)          // владелец И документ активен

	// Процедурная политика для сервисного субъекта.
	importService := rolego.Single(func(_ context.Context, subj string, _ rolego.Perm, _ Document) bool {
		return subj == "import-service"
	})

	ask := func(name string, p rolego.Policy[string, Document]) {
		dec, err := p.Allow(context.Background(), name, PermWrite, doc)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(name, dec.Allow())
	}

	ask("alice", final)          // true — владелец, не исключена
	ask("bob", final)            // false — соавтор, но исключён
	ask("alice", edit)           // true — владелец, документ активен
	ask("import-service", importService) // true — сервисная роль

	// Output:
	// alice true
	// bob false
	// alice true
	// import-service true
}
```

## Ошибки в комбинаторах

- `AllOf`/`Any`/`Except` прерываются на первой ошибке и возвращают `(Deny, err)`: частичного решения нет.
- Ошибка из вложенной политики пробрасывается наверх как есть — не превращается в тихий `Deny`.

## Валидация политики: `Validate()`

Спасительная проверка для целостности ядра — вызывай при старте приложения:

```go
if err := checker.Validate(); err != nil {
	panic(err) // или 500 на старте
}
```

Проверяются: матрица для `Kind` из `Type`, непустая ось, валидное правило комбинации, ненулевые резолвер и экстрактор. Противоречия собираются **все сразу** одной ошибкой через `errors.Join` — каждое находится через `errors.Is`. Подробнее: [Ошибки](errors.md).

Дальше: [Ошибки](errors.md) — как различать и сверять ошибки ядра.
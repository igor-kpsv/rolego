# Скоупы и цепочки

Ресурс почти никогда не «сам по себе»: документ лежит в проекте, проект — в воркспейсе. Роль может быть выдана на любом из этих уровней: «редактор этого документа», «редактор любого документа проекта». rolego называет такой набор уровней **осью** (цепочкой звеньев) и спрашивает резолвер про каждое звено отдельно.

Звено (`Scope`) — просто «адрес» места в иерархии: **тип + номер** («документ №42»). Экстрактор `MapScopes` — твоя «адресная книга»: она объявляет, какие адреса у ресурса.

## Три типа: `Kind`, `Scope`, `Link`

```go
type Kind uint16   // метка типа ресурса: «документ», «проект» — просто числа
type Scope struct {
	Kind Kind
	ID   uint64
}
type Link struct {
	Scope Scope // звено ресурса как есть
	Kind  Kind  // тип звена (равен Scope.Kind)
	Depth int   // 0 — самое глубокое звено (сам ресурс)
}
```

Разница в назначении:

- `Kind` — **что это за тип**. Метка, которую ты объявляешь и сам же используешь везде: «документ», «проект».
- `Scope` — **конкретное звено**. Пара «тип + номер»: «документ №42», «проект №7». Такой парой резолвер различает звенья одного типа.
- `Link` — **звено для твоего резолвера**. То же звено, но в упаковке вызова `RolesAt`: `Scope`, `Kind` и глубина `Depth` — позиция звена на оси (0 — сам ресурс).

## Откуда берётся `Kind` и зачем он в `Scope`

`Kind` придумываешь ты — это твой доменный реестр типов ресурсов:

```go
const (
	KindDocument rolego.Kind = 1 + iota // документ
	KindProject                         // проект
)
```

Числовое значение — просто постоянная метка. Смысл появляется, когда одна и та же метка встречается в трёх местах:

1. в экстракторе — при построении звена `Scope{Kind: KindDocument, ID: ...}`;
2. в матрице — как ключ записи «права для документов»;
3. в оси — как `Level(KindDocument)`.

Через эту метку библиотека связывает звенья ресурса с правами: зная `Kind`, она выбирает матрицу для проверки, а резолвер — решает, какой сценарий применить к звену. Поэтому `Kind` внутри `Scope` обязателен: без него звено безымянно.

## Экстрактор `MapScopes`: ресурс → звенья

Библиотека не знает, как устроен твой `Document`. Ты объясняешь это экстрактором — функция `R → []Scope`. Правило одно: порядок **от корня к ресурсу**, последним — сам ресурс.

```go
func scopes(d Document) []rolego.Scope {
	return []rolego.Scope{
		{Kind: KindProject, ID: d.ProjectID}, // от корня...
		{Kind: KindDocument, ID: d.ID},       // ...к самому документу
	}
}
```

Звеньев может быть сколько угодно — хоть одно (сам документ), хоть десять вложенностей.

## Ось `ScopeChain`: какого порядка уровни и как объединять роли

Ось собирается из `Level` (уровни) и `Combine` (правило). Уровни перечисляются **от самого глубокого к корню**:

```go
chain := rolego.NewScopeChain(
	rolego.Level(KindDocument), // самое глубокое звено — первым
	rolego.Level(KindProject),  // затем родитель
	rolego.Combine(rolego.Nearest),
)
```

Правил комбинации два:

!!! note "Nearest — по умолчанию"
    Берётся первая непустая маска ролей, начиная с самого глубокого звена. На документе у субъекта роли нет — поднимаемся на проект; на проекте нет — на воркспейс.

!!! note "Union"
    Роли всех звеньев объединяются: роль на любом уровне даёт свои права на ресурсе.

Если `Nearest` дошёл до корня и все маски пусты — `Deny` без ошибки. Для `Union` то же: пустое объединение — `Deny`.

Собранная ось хранит порядок и правило; посмотреть их можно методами `Kinds()` и `Rule()`:

```go
chain.Kinds()                    // [KindDocument, KindProject]
chain.Rule() == rolego.Nearest   // true
```

## Глубина: `Depth` в `Link`

`MapScopes` шлёт звенья от корня к ресурсу. Внутри `Check` они обходятся наоборот — от ресурса к корню, и на каждом звене резолвер получает `Link` с `Depth`:

- `Depth: 0` — звено самого ресурса (глубочайшее);
- `Depth: 1` — его родитель;
- и так далее к корню.

`Depth` пригодится, когда роль определена на конкретном уровне оси.

## Резолвер и `Kind`: что решает твоя логика

Резолвер может опираться на `link.Kind` (единые роли по типу) или на `link.Scope.ID` (роль конкретного проекта):

```go
type projectResolver struct {
	members map[uint64][]string // projectID → участники
}

func (r projectResolver) RolesAt(_ context.Context, subj string, link rolego.Link) (rolego.Roles, error) {
	if link.Kind != KindProject {
		return rolego.RolesOf(0), nil // интересуют только звенья проектов
	}
	for _, id := range r.members[link.Scope.ID] {
		if id == subj {
			return rolego.RolesOf(RoleViewer), nil
		}
	}
	return rolego.RolesOf(0), nil
}
```

## Один полный пример: `Nearest` против `Union`

Сюжет: у Алисы на документе роль зрителя, на проекте — редактора. Один и тот же вопрос, два правила оси.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/igor-kpsv/rolego"
)

const (
	RoleViewer  rolego.Role = 1 << iota // зритель
	RoleEditor                          // редактор
)

const (
	PermRead  rolego.Perm = 1 << iota // читать
	PermWrite                          // изменять
)

const (
	KindDocument rolego.Kind = 1 + iota // документ
	KindProject                         // проект
)

type Document struct {
	ID        uint64
	ProjectID uint64
}

func scopes(d Document) []rolego.Scope {
	return []rolego.Scope{
		{Kind: KindProject, ID: d.ProjectID},
		{Kind: KindDocument, ID: d.ID},
	}
}

// kindMembers — резолвер «роль по типу звена из справочника»: роль на любом
// документе одинакова, на любом проекте — своя.
type kindMembers map[rolego.Kind]rolego.Role

func (m kindMembers) RolesAt(_ context.Context, _ string, link rolego.Link) (rolego.Roles, error) {
	return rolego.RolesOf(m[link.Kind]), nil
}

// build собирает Checker с заданным правилом комбинации и резолвером.
func build(rule rolego.ChainRule, res rolego.Resolver[string, Document]) *rolego.Checker[string, Document] {
	c, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](res),
		rolego.WithPolicy[string, Document](
			rolego.Matrices{KindDocument: {
				RoleViewer: PermRead,              // зритель — только читать
				RoleEditor: PermRead | PermWrite,  // редактор — читать и писать
			}},
			rolego.NewScopeChain(
				rolego.Level(KindDocument), // глубже всех — документ
				rolego.Level(KindProject),
				rolego.Combine(rule),
			),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	return c
}

func main() {
	doc := Document{ID: 42, ProjectID: 7}
	// На документе Алиса — зритель, на проекте — редактор.
	members := kindMembers{KindDocument: RoleViewer, KindProject: RoleEditor}

	nearest := build(rolego.Nearest, members)
	union := build(rolego.Union, members)

	dec, _ := nearest.Check(context.Background(), "alice", doc, PermWrite)
	fmt.Println("Nearest:", dec.Allow()) // false — зритель на документе, ближе роли нет

	dec, _ = union.Check(context.Background(), "alice", doc, PermWrite)
	fmt.Println("Union:", dec.Allow()) // true — роль редактора с проекта в сумме

	// Output:
	// Nearest: false
	// Union: true
}
```

## Важное ограничение: ось декларативна

Ось задаёт правило комбинации, но `Check` **не сверяет** звенья из `MapScopes` с декларацией `Level`. Другими словами: если у ресурса случайно не окажется звена проекта, а ось его ждёт — ошибки не будет, просто роли проекта не придут. `Validate()` противоречие между осью и реальными звеньями тоже не поймает. Порядок звеньев в `MapScopes` (от корня к ресурсу) и порядок `Level` (от ресурса к корню) соблюдай сам.

Дальше: [Комбинирование политик](policies.md) — правила, которые нельзя выразить одной матрицей.
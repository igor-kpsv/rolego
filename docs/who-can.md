# Кто имеет право (WhoCan)

Прямой вопрос уже умеет `Check`: «может ли Алиса записать в документ 42?». `WhoCan` отвечает на обратный: **«кто из этого списка может записать в документ 42?»** — полезно для уведомлений, фильтрации списков и разрешений.

## Сигнатура

```go
func (c *Checker[S, R]) WhoCan(ctx context.Context, candidates []S, res R, perm Perm) ([]S, error)
```

Для каждого кандидата выполняется обычный `Check`; в результат попадают те, кто получил `Allow`. Порядок результата повторяет `candidates`, дубликат-кандидат даёт дубликат в результате.

## Один полный пример

Команда сервиса должна узнать себя: «кого можно уведомить об изменении документа 42?» — то есть кто имеет право писать в него.

```go
package main

import (
	"context"
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

type members map[string]rolego.Role

func (m members) RolesAt(_ context.Context, subj string, _ Document, _ rolego.Link) (rolego.Resolved, error) {
	return rolego.Resolved{Roles: rolego.RolesOf(m[subj])}, nil
}

func main() {
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](members{
			"alice": RoleEditor,
			"bob":   RoleViewer,
			"carol": RoleEditor,
		}),
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

	// Кого уведомить: кто из списка команды может писать в документ 42?
	allowed, err := checker.WhoCan(context.Background(),
		[]string{"xavier", "alice", "bob", "carol", "alice"}, doc, PermWrite)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(allowed) // [alice carol alice] — порядок и дубликаты сохранены

	// Output:
	// [alice carol alice]
}
```

## Ошибки

При ошибке `Check` на любом кандидате возвращается `(nil, err)` — частичный результат не выдаётся. Без правды: либо весь список, либо ничего.

## Иерархия ролей

`WhoCan` крутится поверх того же `Check`, поэтому роли, собранные с [`WithHierarchy`](roles.md), учитываются без доп. работы: кандидат со старшей ролью попадает в результат по унаследованным правам предка. Расширенные права живут в матрицах — `WhoCan` смотрит в них, а не в сырые роли.

## Ограничение контракта

`WhoCan` корректно разворачивает **субъект-детерминированную** часть политики: решение по кандидату зависит только от субъекта и ресурса. Если правило зависит ещё и от времени или внешнего состояния (например, «писать можно, пока менеджер не заморозил проект») — `WhoCan` может быть неточен: финальную выборку после него делай сам, наложив дополнительный фильтр.

## Совет по производительности

`WhoCan` — это цикл из `Check`: один кандидат, один проход политики. На больших списках оптимизируй сам резолвер (индексы по ролям субъекта) или его данные (кэш на своей стороне, например в БД) — библиотека решений не кэширует.

Дальше: [Скоупы и цепочки](scopes.md) — сама ось уровней, на которой стоят роли.
# Строки из унаследованной БД (Registry)

Права в типовой базе лежат строками: `documents.view`, `documents.edit`. Мигрировать хранение на биты — трогать схему и данные. `Registry` — мост наоборот: строки из БД конвертятся в биты `Perm`/`Role` на лету, без миграции.

```go
// Registry — именной реестр битов: строковое имя ↔ бит.
// T — твой битовый тип: rolego.Perm или rolego.Role.
type Registry[T ~uint64] struct{ ... }
```

Реестр **бездоменен**: строка для него непрозрачна, будь то `documents.view` или просто `view`. Разбивать её на части он не пытается — домен остаётся у твоего кода.

## Регистрация двумя способами

Оба обязательны — они решают разные задачи.

### `Register` — явный бит уже объявленной константы

Когда право уже заведено константой в коде (`1 << iota`), свяжи её со строкой имени:

```go
const (
	PermRead  rolego.Perm = 1 << iota
	PermWrite
)

var rights = new(rolego.Registry[rolego.Perm])

func init() {
	rights.Register("documents.view", PermRead)
	rights.Register("documents.edit", PermWrite)
}
```

Дубликат имени или бита — паника: рассинхрон реестра с константами кода опаснее паники в тесте.

### `Next` — авто-бит по порядку вызова

Когда бит не важен, а нужна детерминированная нумерация «первая строка — бит 1, вторая — 2»:

```go
var roles = new(rolego.Registry[rolego.Role])

func init() {
	roles.Next("viewer") // 1 << 0
	roles.Next("editor") // 1 << 1
	roles.Next("manager") // 1 << 2
}
```

Порядок совместим с `1 << iota`-объявлениями первых N бит. `Next` пропускает не только биты, точно совпавшие с ключами `Register`, но и биты **внутри составной маски**: `Register("documents.edit", PermRead|PermWrite)` резервирует оба бита, и следующий `Next` выдаст бит за их пределами. Коллизии «`Next` вернул бит, уже живущий внутри другой маски» нет по построению.

## Превращение строк в биты

```go
v, err := roles.Parse("editor")
if err != nil {
	// роль с таким именем не зарегистрирована
}
```

Неизвестное имя — `ErrUnknownRegistryName`, сверяется через `errors.Is`. Для строк, которые обязаны существовать, — `MustParse` (паникует при неизвестном имени).

## Обратно: бит в строку

Для аудит-логов: `audit.Entry.Perm` — бит; читаемое имя даёт `Name`:

```go
name, ok := roles.Name(manager)
if ok {
	log.Printf("менеджер проекта: %s", name)
}
```

Незарегистрированный бит — `ok == false`.

## Один полный пример

Строки из базы правят допуском: у кого в карте есть `documents.edit` — тот редактор.

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

type Document struct{ ID uint64 }

// roleNames — реестр «строка из БД → роль». Строки приходят из унаследованной
// схемы, роли — биты.
var roleNames = new(rolego.Registry[rolego.Role])

func init() {
	roleNames.Register("documents.viewer", RoleViewer)
	roleNames.Register("documents.editor", RoleEditor)
}

// storedResolver читает роль субъекта из старой схемы: строка из карты
// «субъект → роль» переводится в бит реестром roleNames.
type storedResolver map[string]string

func (r storedResolver) RolesAt(_ context.Context, subj string, _ rolego.Link) (rolego.Roles, error) {
	name, ok := r[subj]
	if !ok {
		return rolego.RolesOf(0), nil // нет записи — ролей нет
	}
	return rolego.RolesOf(roleNames.MustParse(name)), nil
}

func main() {
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](func(d Document) []rolego.Scope {
			return []rolego.Scope{{Kind: KindDocument, ID: d.ID}}
		}),
		rolego.Resolve[string, Document](storedResolver{
			"alice": "documents.editor",
			"bob":   "documents.viewer",
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
	fmt.Println(checker.Allows(context.Background(), "alice", doc, PermWrite)) // true — редактор
	fmt.Println(checker.Allows(context.Background(), "bob", doc, PermWrite))    // false — зритель

	// Output:
	// true
	// false
}
```

## Детали

- **Реестр не потокобезопасен.** Заполняется один раз при старте (в `init` или при инициализации приложения), дальше только читается.
- **Нулевое значение рабочее** — отдельный конструктор не нужен.
- **Пустая строка и точка** — не обрабатываются: строка для реестра атомарна.
- **Составные маски резервируют входящие биты.** Пересечение зарегистрированных масок допустимо (`view = PermRead`, `edit = PermRead|PermWrite`), но каждый бит, входящий хотя бы в одну маску, `Next` считает занятым — автонумерация никогда не столкнётся с `Register`.

Дальше: [HTTP-мидлвара (httpx)](wraps/httpx.md) — готовые обёртки для продакшена.
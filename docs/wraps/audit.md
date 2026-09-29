# Аудит (audit)

Обёртка, логирующая **каждое** решение `Check` — для истории доступа и разбора инцидентов. Аудит не влияет на доступ: решение и ошибка возвращаются вызывающему без изменений.

## Сигнатура

```go
func Log[S, R any](checker *rolego.Checker[S, R], logger Logger) *Checker[S, R]

type Logger interface {
	Log(ctx context.Context, e Entry)
}

type Entry struct {
	Subj     any             // субъект проверки
	Res      any             // ресурс
	Perm     rolego.Perm     // запрашиваемое право
	Decision rolego.Decision // Allow или Deny
	Err      error           // nil при успешной проверке
}
```

`Log` оборачивает готовый `Checker`; твой логгер просто получает событие `Entry` на каждую проверку.

## Что делает

```go
audited := audit.Log(checker, myLogger)
decision, err := audited.Check(ctx, user, doc, PermWrite)
// myLogger.Log получает Entry; решение и ошибка — как от обычного Check
```

- Логируются **все** исходы: `Allow`, `Deny` **и** ошибки. Сбой резолвера — тоже событие аудита, а не только 500-й статус.
- Решение возвращается **без изменений** — аудит прозрачен для доступа.
- `WhoCan` и `Validate` делегируются без событий: внутренние проверки `WhoCan` по отдельности не логируются.

## Один полный пример

Логгер печатает субъекта, вердикт и ошибку; Алиса — редактор, Боб — зритель.

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/audit"
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

type members map[string]rolego.Role

func (m members) RolesAt(_ context.Context, subj string, _ rolego.Link) (rolego.Roles, error) {
	return rolego.RolesOf(m[subj]), nil
}

type printLogger struct{}

// Log — приёмник событий. Контракт: логгер не должен паниковать ни при каких
// входных данных, а вызывающий не полагается на его результат.
func (printLogger) Log(_ context.Context, e audit.Entry) {
	fmt.Printf("audit subj=%v allow=%v err=%v\n", e.Subj, e.Decision.Allow(), e.Err)
}

func main() {
	checker, err := rolego.New[string, Document](
		rolego.Type[string, Document](KindDocument),
		rolego.MapScopes[string, Document](scopes),
		rolego.Resolve[string, Document](members{"alice": RoleEditor}),
		rolego.WithPolicy[string, Document](
			rolego.Matrices{KindDocument: {RoleEditor: PermWrite}},
			rolego.NewScopeChain(rolego.Level(KindDocument)),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	audited := audit.Log(checker, printLogger{})
	doc := Document{ID: 42}
	for _, who := range []string{"alice", "bob"} {
		_, _ = audited.Check(context.Background(), who, doc, PermWrite)
	}

	// Output:
	// audit subj=alice allow=true err=<nil>
	// audit subj=bob allow=false err=<nil>
}
```

## Свой логгер: мост к `slog` за пару строк

Обёртка не привязана к конкретному логгеру — достаточно реализовать `Logger`. Мост к стандартному `log/slog`:

```go
type slogAdapter struct{ lg *slog.Logger }

func (a slogAdapter) Log(ctx context.Context, e audit.Entry) {
	level := slog.LevelInfo
	if e.Err != nil {
		level = slog.LevelError
	}
	a.lg.Log(ctx, level, "access check",
		slog.Any("subj", e.Subj),
		slog.Any("res", e.Res),
		slog.Any("perm", e.Perm),
		slog.Any("decision", e.Decision),
		slog.Any("err", e.Err),
	)
}

audited := audit.Log(checker, slogAdapter{lg: slog.Default()})
```

## Нил-логгер

`audit.Log(checker, nil)` падает с паникой: аудит нельзя молча выключить. Если логирование опционально в окружении — передавай no-op-логгер явно.

## Композиция обёрток

Обёртки складываются в цепочку: снаружи аудит, внутри кэш.

```go
cached := caching.Wrap(checker, caching.WithTTL(time.Minute))
audited := audit.Log(cached, slogAdapter{lg: slog.Default()})
```

Каждое решение проходит через кэш (экономит резолвер), но в аудит попадает фактически выданный ответ — включая кэш-хиты.

Дальше: [Ошибки](../errors.md) — различать отказы по политике и технические сбои.
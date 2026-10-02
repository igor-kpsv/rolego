# Подключение к веб-фреймворкам

`httpx.Require` возвращает обычную `func(http.Handler) http.Handler` — ту самую форму, в которой пишут мидлвары `net/http`. Подключение к фреймворку сводится к штатному мосту «фреймворк → net/http», который у каждого уже есть. Отдельные пакеты-адаптеры под каждый фреймворк библиотеке не нужны — и этот раздел показывает, что остаётся у тебя в проекте.

Ниже — шаблоны для Chi, Echo, Gin и Fiber. Фрагменты не компилируются в репозитории (для фреймворков здесь нет зависимостей — свойство rolego), их копируют в проект; `checker`, `perm` и `parse` — из [страницы httpx](httpx.md).

## Chi — из коробки

`Router.Use` принимает именно `func(http.Handler) http.Handler`, поэтому мидлвара встраивается напрямую:

```go
import "github.com/go-chi/chi/v5"

r := chi.NewRouter()
r.Use(httpx.Require(checker, PermWrite, parse))
```

## Echo — `echo.WrapMiddleware`

`echo.MiddlewareFunc` и `func(http.Handler) http.Handler` — разная форма одного понятия; `WrapMiddleware` склеивает их:

```go
import "github.com/labstack/echo/v4"

e := echo.New()
e.Use(echo.WrapMiddleware(httpx.Require(checker, PermWrite, parse)))
```

## Gin — тонкий помощник

`gin.HandlerFunc` несовместим с `func(http.Handler) http.Handler` напрямую: Gin не строит цепочку на `http.Handler`. Мост — фиксированный кусок твоего кода (в отдельный пакет-адаптер его выносить не нужно — он про один метод):

```go
import "github.com/gin-gonic/gin"

// toGin превращает net/http-мидлвару в gin.HandlerFunc. Решения мидлвары
// (включая 400/403/500 от httpx.Require) уходят через c.Writer; при Allow
// следующий обработчик цепочки вызывается через c.Next().
func toGin(mw func(http.Handler) http.Handler) gin.HandlerFunc {
	return func(c *gin.Context) {
		passed := false
		mw(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			passed = true
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)
		if !passed {
			c.Abort() // мидлвара сама ответила и не стала пропускать
		}
	}
}

r := gin.New()
r.Use(toGin(httpx.Require(checker, PermWrite, parse)))
```

## Fiber — входной guard через adaptor

Fiber построен на `fasthttp` и не даёт встроенного `http.Handler`-контекста, поэтому «внутрь» маршрута мидлвару не встроить. Штатный мост `adaptor.HTTPHandler` переводит конечный `http.Handler` в `fiber.Handler` — подходит, когда проверку достаточно сделать на входе приложения:

```go
import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

app := fiber.New()
app.Use(adaptor.HTTPHandler(httpx.Require(checker, PermWrite, parse)(
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // право есть — далее известен только http-контекст
	}),
)))
```

Если маршруту нужен сам `*fiber.Ctx` (а не http-обёртка), проще проверить право собственным хендлером: `httpx.Require` ничем не волшебен — `checker.Check(r.Context(), subj, res, perm)` доступен для вызова из любого кода.

## fasthttp (и любой собственный сервер) — прямой вызов

`httpx.Require` работает только с `net/http`; адаптеров под `fasthttp` в библиотеке нет — и не нужно: готовый адаптер тащил бы чужие зависимости в подпакет ради glue. Суть мидлвари одна, и для собственного сервера это обычный вызов `Check` в начале обработки:

```go
var handler fasthttp.RequestHandler = func(fctx *fasthttp.RequestCtx) {
	subj, res, err := parse(fctx) // твой парсер (subj, res) из запроса
	if err != nil {
		fctx.SetStatusCode(fasthttp.StatusBadRequest) // как httpx.Require: 400
		return
	}
	decision, err := checker.Check(context.Background(), subj, res, PermWrite)
	if err != nil {
		fctx.SetStatusCode(fasthttp.StatusInternalServerError) // сбой, а не отказ: 500
		return
	}
	if !decision.Allow() {
		fctx.SetStatusCode(fasthttp.StatusForbidden) // 403
		return
	}
	// право есть — продолжаем обработку
}

server := &fasthttp.Server{Handler: handler}
```

Семантика статусов — та же, что у `httpx.Require`: 400 (парсер), 500 (сбой проверки), 403 (`Deny`). Если хочется различать причины `Deny` (например, «403 нет доступа» vs «403 недостаточно прав») — используй [`CheckResult`](../check.md) вместо `Check`.

## Общий приём

Любой фреймворк, у которого нет готового моста, обслуживается одним и тем же фиксированным шаблоном: отдай мидлваре `http.Handler`, который на `Allow` продолжает цепочку фреймворка (Chi/Echo — штатно, Gin — `c.Next()`, Fiber — через `adaptor`). Это 5–6 строк glue в потребительском коде; перекладывать их в библиотеку нет причины — реализации фреймворков живут чужие релизные циклы, а свойство rolego «ноль внешних зависимостей» стоит дороже.
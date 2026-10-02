package rolego

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Checker — ролевая авторизация по битовым маскам: экстрактор звеньев ресурса
// (MapScopes) → роли субъекта на звеньях (Resolver) → комбинация масок по оси
// (ScopeChain) → проверка права по матрице (Matrices).
type Checker[S, R any] struct {
	kind      Kind
	mapScopes func(R) []Scope
	resolver  Resolver[S, R]
	mats      Matrices
	chain     ScopeChain
}

// Option — опция сборки Checker через New; каждая опция записывает одно поле
// конфигурации. Повторный вызов одной и той же опции перезаписывает значение
// (последний вызов побеждает).
type Option[S, R any] func(*config[S, R]) error

// config — состояние сборки Checker, передаваемое опциям.
type config[S, R any] struct {
	kind      Kind
	mapScopes func(R) []Scope
	resolver  Resolver[S, R]
	mats      Matrices
	chain     ScopeChain
	hierarchy Hierarchy
}

// New собирает Checker, применяя опции слева направо; при ошибке любой опции
// возвращает nil и эту ошибку. nil-опция в списке молча пропускается. Если
// задана WithHierarchy, после опций иерархия разворачивается в копии матриц
// (expandHierarchy); ошибка этого шага — например ErrHierarchyCycle при цикле —
// тоже возвращается как (nil, err). Валидация политики (полнота матриц, ось,
// nil резолвер/экстрактор) выполняется на Этапе 6 в Validate, здесь не проводится.
func New[S, R any](opts ...Option[S, R]) (*Checker[S, R], error) {
	var cfg config[S, R]
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return nil, err
		}
	}
	mats := cfg.mats
	if len(cfg.hierarchy) > 0 {
		var err error
		mats, err = expandHierarchy(cfg.mats, cfg.hierarchy)
		if err != nil {
			return nil, err
		}
	}
	return &Checker[S, R]{
		kind:      cfg.kind,
		mapScopes: cfg.mapScopes,
		resolver:  cfg.resolver,
		mats:      mats,
		chain:     cfg.chain,
	}, nil
}

// Type задаёт Kind ресурса: итоговая маска проверяется по матрице именно этого
// Kind; повторный вызов перезаписывает Kind.
func Type[S, R any](kind Kind) Option[S, R] {
	return func(c *config[S, R]) error {
		c.kind = kind
		return nil
	}
}

// MapScopes задаёт экстрактор звеньев ресурса (Вариант A): res → звенья от корня
// к самому ресурсу; повторный вызов перезаписывает экстрактор.
func MapScopes[S, R any](f func(R) []Scope) Option[S, R] {
	return func(c *config[S, R]) error {
		c.mapScopes = f
		return nil
	}
}

// Resolve задаёт резолвер ролей субъекта; повторный вызов перезаписывает резолвер.
func Resolve[S, R any](r Resolver[S, R]) Option[S, R] {
	return func(c *config[S, R]) error {
		c.resolver = r
		return nil
	}
}

// WithPolicy задаёт матрицы прав и ось цепочки как единую политику; повторный
// вызов перезаписывает и матрицы, и ось. mats хранится по ссылке без копии:
// мутация карты после New при одновременном Check — гонка по данным.
//
// Имя отличается от Policy-интерфейса (policy.go): одноимённые тип и функция в
// пакете несовместимы.
func WithPolicy[S, R any](mats Matrices, chain ScopeChain) Option[S, R] {
	return func(c *config[S, R]) error {
		c.mats = mats
		c.chain = chain
		return nil
	}
}

// Check проверяет право perm субъекта subj на ресурс res:
// MapScopes → звенья, RolesAt на каждом звене (от самого глубокого к корню) →
// маски ролей, combine по правилу оси; гранты OR-ятся по всем звеньям. Итог:
// (Perms(merged) | grantsAll) & perm != 0 — Allow.
// perm == 0 — (Deny, ErrZeroPerm); нет звеньев, пустые роли и гранты — Deny;
// ошибка резолвера — (Deny, err). Причину отказа несёт CheckResult.
func (c *Checker[S, R]) Check(ctx context.Context, subj S, res R, perm Perm) (Decision, error) {
	r, err := c.CheckResult(ctx, subj, res, perm)
	return r.Decision, err
}

// CheckResult — Check с причиной отказа: решение и (при Deny) DenyReason.
// Порядок логики:
//
//  1. perm == 0 → (Deny, ErrZeroPerm), Reason = DenyReasonNoPerm.
//  2. Ошибка конвейера (резолвер) → (Deny, err), Reason незначим.
//  3. Роли пусты и грантов нет → (Deny, DenyReasonNoRoles), nil err.
//  4. effective := mats[kind].Perms(roles) | grants; effective&perm != 0 → Allow.
//  5. Иначе → (Deny, DenyReasonNoPerm), nil err.
func (c *Checker[S, R]) CheckResult(ctx context.Context, subj S, res R, perm Perm) (Result, error) {
	if perm == 0 {
		return Result{Decision: Deny, Reason: DenyReasonNoPerm}, fmt.Errorf("%w: %d", ErrZeroPerm, perm)
	}

	got, err := c.collect(ctx, subj, res)
	if err != nil {
		return Result{Decision: Deny}, err
	}

	if got.roles.bits == 0 && got.grants == 0 {
		return Result{Decision: Deny, Reason: DenyReasonNoRoles}, nil
	}

	effective := c.mats[c.kind].Perms(got.roles) | got.grants
	if effective&perm != 0 {
		return Result{Decision: Allow}, nil
	}
	return Result{Decision: Deny, Reason: DenyReasonNoPerm}, nil
}

// RolesFor возвращает маску ролей субъекта subj на ресурсе res: тот же конвейер,
// что в Check (MapScopes → RolesAt по звеньям → combine по правилу оси), но без
// сверки с матрицей прав и без учёта грантов — персональные права (Resolved.Grants)
// в возвращаемую маску не входят. Удобно спрашивать «какие роли у субъекта на этом
// ресурсе?» (владелец, менеджер скоупа), не заводя простановочные биты в матрице.
// Ошибка резолвера — (RolesOf(0), err); нет звеньев или пустые маски — пустое
// множество ролей, ошибки нет.
//
// Иерархия ролей (WithHierarchy) в возвращаемой маске не раскрывается: она
// расширяет только матрицы прав при сборке Checker и не влияет на RolesAt;
// маска — ровно то, что выдал резолвер на звеньях ресурса.
func (c *Checker[S, R]) RolesFor(ctx context.Context, subj S, res R) (Roles, error) {
	got, err := c.collect(ctx, subj, res)
	if err != nil {
		return Roles{}, err
	}
	return got.roles, nil
}

// scoped — результат общего конвейера проверки: итоговая маска ролей по правилу
// оси (roles) и суммарные персональные права (grants) — OR по всем звеньям,
// независимо от правила оси.
type scoped struct {
	roles  Roles
	grants Perm
}

// collect — общая часть CheckResult и RolesFor: звенья ресурса от MapScopes →
// маска ролей и гранты на каждом звене от резолвера → объединение ролей по
// правилу оси; гранты OR-ятся по всем звеньям (аддитивная надбавка к правам
// ролей) и собираются даже при пустой итоговой маске ролей. Пустой список
// звеньев — нули. Ошибка резолвера — (scoped{}, err).
func (c *Checker[S, R]) collect(ctx context.Context, subj S, res R) (scoped, error) {
	scopes := c.mapScopes(res)
	if len(scopes) == 0 {
		return scoped{}, nil
	}

	// Звенья от MapScopes идут от корня (индекс 0) к самому ресурсу (последний).
	// depth — позиция звена в оси, где 0 у самого глубокого (ресурса). Маски
	// укладываем по индексу depth, так что masks[0] — маска ресурса: это соглашение
	// принимает combine (индекс 0 — самое глубокое звено). Идём от ресурса к корню.
	var grants Perm
	masks := make([]Roles, len(scopes))
	for j := len(scopes) - 1; j >= 0; j-- {
		scope := scopes[j]
		depth := len(scopes) - 1 - j
		got, err := c.resolver.RolesAt(ctx, subj, res, Link{Scope: scope, Kind: scope.Kind, Depth: depth})
		if err != nil {
			return scoped{}, err
		}
		masks[depth] = got.Roles
		grants |= got.Grants
	}

	merged, ok := combine(c.chain.Rule(), masks)
	if !ok {
		merged = RolesOf(0)
	}
	return scoped{roles: merged, grants: grants}, nil
}

// Validate проверяет целостность политики: матрица для c.kind
// (ErrNoMatrixForKind), непустая ось (ErrEmptyChain), валидное правило
// комбинации (ErrInvalidChainRule), не-nil резолвер (ErrNilResolver) и экстрактор
// (ErrNilMapScopes). Противоречия собираются все сразу и возвращаются одним
// errors.Join — каждый компонент своя sentinel-ошибка, поэтому сверяются через
// errors.Is. При отсутствии противоречий — nil.
func (c *Checker[S, R]) Validate() error {
	var errs []error
	if _, ok := c.mats[c.kind]; !ok {
		errs = append(errs, ErrNoMatrixForKind)
	}
	if len(c.chain.kinds) == 0 {
		errs = append(errs, ErrEmptyChain)
	}
	if c.chain.rule > Union {
		errs = append(errs, ErrInvalidChainRule)
	}
	if nilValue(c.resolver) {
		errs = append(errs, ErrNilResolver)
	}
	if nilValue(c.mapScopes) {
		errs = append(errs, ErrNilMapScopes)
	}
	return errors.Join(errs...)
}

// WhoCan возвращает тех из candidates, кто имеет право perm на ресурсе res:
// для каждого кандидата выполняется Check; при Allow кандидат попадает в
// результат. Порядок результата повторяет candidates (дубликат-кандидат даёт
// дубликат в результате). При ошибке Check на любом кандидате возвращается
// (nil, err) — частичный результат не отдаётся.
//
// Ограничение контракта: WhoCan разворачивает только субъект-детерминированную
// часть политики. Правила, где право зависит ещё и от ресурса/времени
// (например, «открыть можно, пока дверь не заблокирована менеджером»),
// разворачиваются WhoCan неточно — финальную выборку после WhoCan делает
// вызывающий.
func (c *Checker[S, R]) WhoCan(ctx context.Context, candidates []S, res R, perm Perm) ([]S, error) {
	allowed := make([]S, 0, len(candidates))
	for _, cand := range candidates {
		dec, err := c.Check(ctx, cand, res, perm)
		if err != nil {
			return nil, err
		}
		if dec.Allow() {
			allowed = append(allowed, cand)
		}
	}
	return allowed, nil
}

// nilValue сообщает, является ли v «настоящим» nil, на котором вызов упадёт:
// nil-интерфейс, типизированный nil-указатель или nil-функция. Значения, чьё
// нулевое состояние безопасно для чтения (например, nil-мапа карты-резолвера:
// чтение по ключу вернёт нулевое значение, а не упадёт), nil-значением не
// считаются — обычная проверка v == nil их и так пропускает.
func nilValue(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Pointer:
		return rv.IsNil()
	}
	return false
}

// Allows — bool-обёртка над Check для коротких вызовов: true тогда и только тогда,
// когда правая проверка вернула Allow без ошибки. Ошибка (включая ErrZeroPerm)
// трактуется как Deny и даёт false — для случаев, где важна причина отказа,
// используйте Check напрямую.
func (c *Checker[S, R]) Allows(ctx context.Context, subj S, res R, perm Perm) bool {
	dec, err := c.Check(ctx, subj, res, perm)
	if err != nil {
		return false
	}
	return dec.Allow()
}

// Package caching — кэширующая обёртка над rolego.Checker: решения Check
// кэшируются по ключу (субъект, ресурс, право), ошибочные проверки не
// кэшируются. Ключ построен на самом ресурсе, а не на его звеньях: ядро не
// экспортирует звенья (поле mapScopes приватное), поэтому обёртка требует
// comparable-ресурс и полагается на детерминизм резолвера и экстрактора — это и
// так контракт ядра. Тем самым пара (scope, perm) из концепта покрыта здесь
// ресурсом res: для детерминированного резолвера res — агрегат своих звеньев.
package caching

import (
	"context"
	"sync"
	"time"

	"github.com/igor-kpsv/rolego"
)

// Checker — кэширующая обёртка над rolego.Checker: Check сначала консультирует
// кэш, WhoCan и Validate делегируются подлинному checker без кэша.
type Checker[S comparable, R comparable] struct {
	inner   *rolego.Checker[S, R]
	mu      sync.Mutex
	entries map[key[S, R]]cacheEntry
	order   []key[S, R] // порядок вставки, FIFO для эвикции
	ttl     time.Duration
	max     int
}

// key — ключ кэша: входы Check целиком.
type key[S comparable, R comparable] struct {
	subj S
	res  R
	perm rolego.Perm
}

// cacheEntry — запись кэша: решение успешной проверки и срок жизни; нулевой
// expires — бессрочная запись.
type cacheEntry struct {
	dec     rolego.Decision
	expires time.Time
}

// Option — опция настройки Checker через Wrap; применяется слева направо,
// повторный вызов той же опции перезаписывает значение.
type Option func(*config) error

// config — состояние настройки Checker, передаваемое опциям.
type config struct {
	ttl time.Duration
	max int
}

// Wrap возвращает кэширующую обёртку над checker: решения Check кэшируются по
// ключу (subj, res, perm). S и R обязаны быть comparable — key строится на самом
// ресурсе, так как ядро не экспортирует звенья ресурса. Ошибки проверки не
// кэшируются: каждая ошибочная проверка заново опрашивает подлинный checker.
// Значения опций вне допустимого диапазона означают «выключено» и ошибки не
// возвращают.
func Wrap[S comparable, R comparable](checker *rolego.Checker[S, R], opts ...Option) *Checker[S, R] {
	var cfg config
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		_ = opt(&cfg)
	}
	return &Checker[S, R]{
		inner:   checker,
		entries: make(map[key[S, R]]cacheEntry),
		ttl:     cfg.ttl,
		max:     cfg.max,
	}
}

// WithMaxEntries — лимит числа записей кэша: при превышении вытесняется самая
// старая запись (FIFO по порядку вставки). n <= 0 — лимита нет.
func WithMaxEntries(n int) Option {
	return func(c *config) error {
		c.max = n
		return nil
	}
}

// WithTTL — срок жизни записи кэша с момента создания; по истечении запись
// считается промахом и заменяется свежей. d <= 0 — без срока жизни.
func WithTTL(d time.Duration) Option {
	return func(c *config) error {
		c.ttl = d
		return nil
	}
}

// Check проверяет право perm через кэш: при попадании возвращает записанное
// решение без опроса резолвера; при промахе вызывает подлинный checker и
// сохраняет только успешное решение (ошибка не кэшируется и возвращается как есть).
func (c *Checker[S, R]) Check(ctx context.Context, subj S, res R, perm rolego.Perm) (rolego.Decision, error) {
	k := key[S, R]{subj: subj, res: res, perm: perm}
	now := time.Now()

	c.mu.Lock()
	e, ok := c.entries[k]
	if ok && (c.ttl == 0 || now.Before(e.expires)) {
		c.mu.Unlock()
		return e.dec, nil
	}
	if ok {
		delete(c.entries, k) // запись истекла — промах
	}
	c.mu.Unlock()

	dec, err := c.inner.Check(ctx, subj, res, perm)
	if err != nil {
		return rolego.Deny, err
	}

	c.mu.Lock()
	c.storeLocked(k, dec, now)
	c.mu.Unlock()
	return dec, nil
}

// storeLocked вносит успешное решение; вызывающий держит c.mu.
func (c *Checker[S, R]) storeLocked(k key[S, R], dec rolego.Decision, now time.Time) {
	c.purgeLocked(now)
	e := cacheEntry{dec: dec}
	if c.ttl > 0 {
		e.expires = now.Add(c.ttl)
	}
	if _, ok := c.entries[k]; !ok {
		c.order = append(c.order, k)
	}
	c.entries[k] = e
	c.applyLimitLocked()
}

// purgeLocked удаляет истекшие записи из карты; устаревшие ключи остаются в order
// и пропускаются эвикцией. Вызывающий держит c.mu.
func (c *Checker[S, R]) purgeLocked(now time.Time) {
	if c.ttl == 0 {
		return
	}
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
		}
	}
}

// applyLimitLocked сводит число записей к пределу FIFO-эвикцией. Вызывающий
// держит c.mu.
func (c *Checker[S, R]) applyLimitLocked() {
	for c.max > 0 && len(c.entries) > c.max {
		c.evictFrontLocked()
	}
}

// evictFrontLocked удаляет самую старую живую запись. Вызывающий держит c.mu.
func (c *Checker[S, R]) evictFrontLocked() {
	for len(c.order) > 0 {
		k := c.order[0]
		c.order = c.order[1:]
		if _, ok := c.entries[k]; ok {
			delete(c.entries, k)
			return
		}
	}
}

// WhoCan делегирует подлинному checker: кэш решений на WhoCan не влияет.
func (c *Checker[S, R]) WhoCan(ctx context.Context, candidates []S, res R, perm rolego.Perm) ([]S, error) {
	return c.inner.WhoCan(ctx, candidates, res, perm)
}

// Validate делегирует подлинному checker: кэш на целостность политики не влияет.
func (c *Checker[S, R]) Validate() error {
	return c.inner.Validate()
}

// Clear — полная ручная инвалидация: удаляет все записи кэша.
func (c *Checker[S, R]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[key[S, R]]cacheEntry)
	c.order = nil
}

// Len возвращает текущее число живых записей кэша (истекшие при этом удаляются).
func (c *Checker[S, R]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.purgeLocked(time.Now())
	return len(c.entries)
}

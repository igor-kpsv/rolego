// Package audit — обёртка над rolego.Checker: каждое решение Check логируется
// через Logger, решение/ошибка возвращаются вызывающему без изменений.
package audit

import (
	"context"

	"github.com/igor-kpsv/rolego"
)

// Entry — аудит-событие одного решения Check.
type Entry struct {
	Subj     any
	Res      any
	Perm     rolego.Perm
	Decision rolego.Decision // Allow или Deny
	Err      error           // nil при успешной проверке
}

// Logger — приёмник аудит-событий. Контракт: Log не должен паниковать при
// любых входных данных; вызывающий не полагается на результат.
type Logger interface {
	Log(ctx context.Context, e Entry)
}

// Checker — аудит-обёртка над rolego.Checker: Check логирует решение через
// Logger и возвращает его без изменений; WhoCan и Validate делегируются.
type Checker[S, R any] struct {
	inner *rolego.Checker[S, R]
	log   Logger
}

// Log оборачивает checker: каждое решение Check передаётся в logger (включая
// ошибочные: сбой резолвера — тоже событие для аудита). Возвращённое решение и
// ошибка идентичны исходным. logger обязан быть не nil (иначе паника — это
// ошибка программиста: аудит нельзя молча выключить).
func Log[S, R any](c *rolego.Checker[S, R], logger Logger) *Checker[S, R] {
	if logger == nil {
		panic("audit: nil logger")
	}
	return &Checker[S, R]{inner: c, log: logger}
}

// Check проверяет право perm делегированием подлинному checker и логирует
// решение в logger — и успешное, и ошибочное (сбой резолвера — тоже событие
// аудита). Решение и ошибка возвращаются без изменений.
func (c *Checker[S, R]) Check(ctx context.Context, subj S, res R, perm rolego.Perm) (rolego.Decision, error) {
	dec, err := c.inner.Check(ctx, subj, res, perm)
	c.log.Log(ctx, Entry{Subj: subj, Res: res, Perm: perm, Decision: dec, Err: err})
	return dec, err
}

// WhoCan делегирует подлинному checker; внутренние проверки WhoCan не логируются
// по отдельности — финальный результат WhoCan возвращается как есть.
func (c *Checker[S, R]) WhoCan(ctx context.Context, candidates []S, res R, perm rolego.Perm) ([]S, error) {
	return c.inner.WhoCan(ctx, candidates, res, perm)
}

// Validate делегирует подлинному checker: целостность политики аудиту не логируется.
func (c *Checker[S, R]) Validate() error {
	return c.inner.Validate()
}

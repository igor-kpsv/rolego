package rolego

import "context"

// Policy — правило допуска субъекта subj к ресурсу res на право perm.
type Policy[S, R any] interface {
	Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error)
}

// Single конвертит bool-предикат f в самостоятельную процедурную политику без ролей.
func Single[S, R any](f func(context.Context, S, Perm, R) bool) Policy[S, R] {
	return predicatePolicy[S, R](f)
}

// Predicate конвертит bool-предикат f в условие внутри политики: true — Allow, false — Deny.
func Predicate[S, R any](f func(context.Context, S, Perm, R) bool) Policy[S, R] {
	return predicatePolicy[S, R](f)
}

// predicatePolicy — общая реализация Single и Predicate: true → Allow, false → Deny.
type predicatePolicy[S, R any] func(context.Context, S, Perm, R) bool

// Allow вызывает предикат и превращает bool-результат в Decision.
func (f predicatePolicy[S, R]) Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error) {
	if f(ctx, subj, perm, res) {
		return Allow, nil
	}
	return Deny, nil
}

// AllOf разрешает, если все политики ps разрешили (пустой список — вакуумно истинно).
func AllOf[S, R any](ps ...Policy[S, R]) Policy[S, R] {
	return allPolicy[S, R](ps)
}

// allPolicy — комбинатор AllOf: первая ошибка или Deny прерывают обход (короткое замыкание).
type allPolicy[S, R any] []Policy[S, R]

// Allow обходит политики слева направо и требует Allow от каждой.
func (ps allPolicy[S, R]) Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error) {
	for _, p := range ps {
		d, err := p.Allow(ctx, subj, perm, res)
		if err != nil {
			return Deny, err
		}
		if !d.Allow() {
			return Deny, nil
		}
	}
	return Allow, nil
}

// Any разрешает, если хотя бы одна политика ps разрешила (пустой список — вакуумно ложно).
func Any[S, R any](ps ...Policy[S, R]) Policy[S, R] {
	return anyPolicy[S, R](ps)
}

// anyPolicy — комбинатор Any: первое Allow или первая ошибка прерывают обход.
type anyPolicy[S, R any] []Policy[S, R]

// Allow обходит политики слева направо и разрешает по первому Allow.
func (ps anyPolicy[S, R]) Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error) {
	for _, p := range ps {
		d, err := p.Allow(ctx, subj, perm, res)
		if err != nil {
			return Deny, err
		}
		if d.Allow() {
			return Allow, nil
		}
	}
	return Deny, nil
}

// Except повторяет base, если ни одна из denied не разрешила (deny-wins:
// базовое Allow сводится на нет любым Allow из denied).
func Except[S, R any](base Policy[S, R], denied ...Policy[S, R]) Policy[S, R] {
	return exceptPolicy[S, R]{base: base, denied: denied}
}

// exceptPolicy — комбинатор Except: deny-wins, любая ошибка пропагируется наверх.
type exceptPolicy[S, R any] struct {
	base   Policy[S, R]
	denied []Policy[S, R]
}

// Allow сначала спрашивает base, затем проверяет, что ни одна denied не разрешила.
func (e exceptPolicy[S, R]) Allow(ctx context.Context, subj S, perm Perm, res R) (Decision, error) {
	d, err := e.base.Allow(ctx, subj, perm, res)
	if err != nil {
		return Deny, err
	}
	if !d.Allow() {
		return Deny, nil
	}
	for _, p := range e.denied {
		d, err := p.Allow(ctx, subj, perm, res)
		if err != nil {
			return Deny, err
		}
		if d.Allow() {
			return Deny, nil
		}
	}
	return Allow, nil
}

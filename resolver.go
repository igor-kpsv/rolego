package rolego

import "context"

// Resolver — источник ролей субъекта на одном звене оси: для каждого звена
// возвращает маску ролей, которые субъект subj имеет на этом звене.
type Resolver[S, R any] interface {
	// RolesAt возвращает роли субъекта subj на звене link; ошибку (например,
	// сбой хранилища ролей) Check возвращает наверх как (Deny, err).
	RolesAt(ctx context.Context, subj S, link Link) (Roles, error)
}

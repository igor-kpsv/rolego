package rolego

import "context"

// Resolved — ответ резолвера на одном звене: роли субъекта и персональные
// права (гранты). Grants — аддитивная маска Perm, добавляемая к правам ролей
// по OR; ноль — «грантов нет».
type Resolved struct {
	Roles  Roles
	Grants Perm
}

// Resolver — источник данных о субъекте на звене: для каждого звена возвращает
// роли и персональные права. res — сам ресурс: роли/гранты могут зависеть от
// любого его поля (например, UUID-идентификатора), а не только от числового Scope.ID.
type Resolver[S, R any] interface {
	// RolesAt возвращает роли субъекта subj и его персональные права (гранты)
	// на звене link. Ошибку (например, сбой хранилища ролей) Check возвращает
	// наверх как (Deny, err).
	RolesAt(ctx context.Context, subj S, res R, link Link) (Resolved, error)
}

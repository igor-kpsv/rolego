package rolego

// Hierarchy — декларация старшинства ролей: роль → маска родительских ролей.
// Дочерняя роль включает права родителей (и транзитивно всё, что включают
// те). Маска родителя может быть составной: роль наследует нескольких предков.
//
// Расширение происходит на этапе сборки Checker (разворачивается в матрицы),
// а не в момент Check, поэтому иерархия не влияет на резолвер, ось и подпакеты.
type Hierarchy map[Role]Role

// WithHierarchy задаёт иерархию ролей субъекта при сборке Checker: права,
// которые родительские роли дают в каждой матрице, транзитивно добавляются
// дочерним ролям. Роли, участвующие в иерархии, но не имеющие собственной
// записи в матрице, тоже получают права предков (например, объявленный в
// иерархии менеджер на матрице, где выписаны только зритель и редактор).
// Порядок опций неважен: расширение выполняется после всех опций в New.
// Цикл в графе — ошибка New (ErrHierarchyCycle); пустая или nil иерархия — no-op.
//
// Работает поверх WithPolicy: исходные матрицы не мутируются, нормализованная
// копия отдаётся Checker'у.
func WithHierarchy[S, R any](h Hierarchy) Option[S, R] {
	return func(c *config[S, R]) error {
		c.hierarchy = h
		return nil
	}
}

// expandHierarchy разворачивает иерархию ролей в копию матриц: для каждой роли
// — и ключа матрицы, и любого узла графа иерархии — к её собственным правам
// добавляются права всех предков (транзитивно). Роль без своей записи в матрице
// приобретает только унаследованные права (записью с нулевым правом не раздуваемся).
// Цикл в графе — (nil, ErrHierarchyCycle).
func expandHierarchy(mats Matrices, h Hierarchy) (Matrices, error) {
	hierarchyRoles := collectHierarchyRoles(h)

	out := make(Matrices, len(mats))
	for kind, m := range mats {
		nm := make(Matrix, len(m)+len(hierarchyRoles))
		for role, perm := range m {
			nm[role] = perm
		}
		for role := range hierarchyRoles {
			inherited, err := ancestorPerms(m, h, role, make(map[Role]bool))
			if err != nil {
				return nil, err
			}
			if inherited == 0 {
				continue
			}
			nm[role] = m[role] | inherited
		}
		out[kind] = nm
	}
	return out, nil
}

// collectHierarchyRoles возвращает все роли, участвующие в иерархии — ключи
// (дети) и все биты значений (родители).
func collectHierarchyRoles(h Hierarchy) map[Role]bool {
	roles := make(map[Role]bool, len(h)*2)
	for child, parents := range h {
		roles[child] = true
		for parents != 0 {
			bit := parents & -parents
			parents &^= bit
			roles[Role(bit)] = true
		}
	}
	return roles
}

// ancestorPerms собирает права, которые роль role получает от предков по
// иерархии h: для каждого родителя — его собственные права в m плюс права его
// предков (рекурсивно). visiting — путь текущего обхода для детекта цикла.
func ancestorPerms(m Matrix, h Hierarchy, role Role, visiting map[Role]bool) (Perm, error) {
	if visiting[role] {
		return 0, ErrHierarchyCycle
	}
	parents := h[role]
	if parents == 0 {
		return 0, nil
	}
	visiting[role] = true
	defer delete(visiting, role)

	var sum Perm
	for parents != 0 {
		bit := parents & -parents
		parents &^= bit
		parent := Role(bit)
		inherited, err := ancestorPerms(m, h, parent, visiting)
		if err != nil {
			return 0, err
		}
		sum |= inherited | m[parent]
	}
	return sum, nil
}

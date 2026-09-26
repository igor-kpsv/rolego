package rolego

// Matrix — права на тип ресурса: какая роль какие биты Perm выдаёт.
type Matrix map[Role]Perm

// Matrices — матрицы прав по типам ресурса; бит права одного Kind не переносится на другой.
type Matrices map[Kind]Matrix

// Perms возвращает OR прав всех ролей матрицы, присутствующих в маске roles.
func (m Matrix) Perms(roles Roles) Perm {
	var sum Perm
	for role, right := range m {
		if roles.Has(role) {
			sum |= right
		}
	}
	return sum
}

// Allow проверяет, что запрошенное право perm выдано хотя бы одной роли из roles.
func (m Matrix) Allow(roles Roles, perm Perm) bool {
	return m.Perms(roles)&perm != 0
}

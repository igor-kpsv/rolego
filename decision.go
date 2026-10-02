package rolego

// Decision — результат проверки права: Deny или Allow.
type Decision uint8

const (
	// Deny — доступ запрещён.
	Deny Decision = iota
	// Allow — доступ разрешён.
	Allow
)

// Allow проверяет, что решение положительное (Allow).
func (d Decision) Allow() bool { return d == Allow }

// DenyReason — причина отказа; валидна при Decision == Deny.
type DenyReason uint8

const (
	// DenyReasonNoRoles — на звеньях нет ролей / итоговая маска ролей пуста,
	// а персональных грантов нет («нет доступа к ресурсу»).
	DenyReasonNoRoles DenyReason = iota

	// DenyReasonNoPerm — роли (или гранты) есть, но право не выписано ни
	// матрицей, ни грантами («недостаточно прав»).
	DenyReasonNoPerm
)

// Result — решение Check с причиной отказа. Ошибка резолвера остаётся во
// втором возвращаемом значении CheckResult; здесь её поля нет.
type Result struct {
	Decision Decision
	Reason   DenyReason // смысл имеет только при Deny
}

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

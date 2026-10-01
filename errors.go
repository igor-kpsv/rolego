package rolego

import "errors"

// Sentinel-ошибки ядра. Все ошибки рантайма оборачивают одну из них через %w,
// поэтому сверять их нужно через errors.Is, а не ==.
var (
	// ErrEmptyChain — ось ScopeChain пуста (нет ни одного звена).
	ErrEmptyChain = errors.New("rolego: empty scope chain")

	// ErrInvalidChainRule — правило комбинации оси не равно Nearest или Union.
	ErrInvalidChainRule = errors.New("rolego: invalid chain rule")

	// ErrNoMatrixForKind — в Matrices нет матрицы для Kind, заданного в Type.
	ErrNoMatrixForKind = errors.New("rolego: no matrix for kind")

	// ErrNilResolver — переданный резолвер равен nil.
	ErrNilResolver = errors.New("rolego: nil resolver")

	// ErrNilMapScopes — переданный экстрактор MapScopes равен nil.
	ErrNilMapScopes = errors.New("rolego: nil MapScopes")

	// ErrZeroPerm — запрошенное право равно нулю.
	ErrZeroPerm = errors.New("rolego: zero perm requested")

	// ErrHierarchyCycle — граф иерархии ролей содержит цикл (роль включает
	// себя напрямую или через цепочку предков).
	ErrHierarchyCycle = errors.New("rolego: cycle in role hierarchy")

	// ErrUnknownRegistryName — строковое имя не зарегистрировано в Registry.
	ErrUnknownRegistryName = errors.New("rolego: unknown registry name")
)

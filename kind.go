package rolego

// Kind — тип ресурса; для ядра это просто число (домен — у пользователя).
type Kind uint16

// Scope — звено ресурса: тип и идентификатор.
type Scope struct {
	Kind Kind
	ID   uint64
}

// Link — звено исполнения для резолвера: скоуп звена из ресурса, kind самого
// звена (равен scope.Kind) и глубина от ресурса (0 — звено самого ресурса).
type Link struct {
	Scope Scope
	Kind  Kind
	Depth int
}

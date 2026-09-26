package rolego

// ChainRule — правило комбинации масок ролей по цепочке.
type ChainRule uint8

const (
	// Nearest — первая непустая маска от самого глубокого звена; все пусто — deny.
	Nearest ChainRule = iota
	// Union — OR всех масок; итог пуст — deny.
	Union
)

// levelSpec — опция-маркер сборки оси: одно звено.
type levelSpec struct{ kind Kind }

// combineSpec — опция-маркер сборки оси: правило комбинации.
type combineSpec struct{ rule ChainRule }

// Level возвращает опцию-маркер для NewScopeChain: одно звено оси.
func Level(kind Kind) any { return levelSpec{kind} }

// Combine возвращает опцию-маркер для NewScopeChain: правило комбинации масок.
// Валидные значения — Nearest, Union; невалидное значение падает в Validate
// (ErrInvalidChainRule), а не в момент комбинации.
func Combine(rule ChainRule) any { return combineSpec{rule} }

// ScopeChain — ось цепочки: декларированный порядок типов звеньев и правило
// комбинации. Правило используется в Check; набор звеньев (kinds) — только
// декларация: в рантайме Check не сверяет его со скопами из MapScopes, так что
// расхождение «ось ↔ скопы у ресурса» Validate не поймает.
type ScopeChain struct {
	kinds []Kind
	rule  ChainRule
}

// NewScopeChain собирает ось из опций Level и Combine: порядок Level задаёт звенья
// от самого глубокого к корню; повторный Combine — последний выигрывает; без
// Combine правило по умолчанию Nearest. Порядок kinds доступен через Kinds, но
// служит декларацией: Check его не сверяет со скопами из MapScopes.
func NewScopeChain(levels ...any) ScopeChain {
	var c ScopeChain
	for _, opt := range levels {
		switch o := opt.(type) {
		case levelSpec:
			c.kinds = append(c.kinds, o.kind)
		case combineSpec:
			c.rule = o.rule
		}
	}
	return c
}

// Kinds возвращает порядок типов звеньев: от звена ресурса (индекс 0) к родителю; результат — копия, мутации вызывающего не влияют на ось.
func (c ScopeChain) Kinds() []Kind {
	got := make([]Kind, len(c.kinds))
	copy(got, c.kinds)
	return got
}

// Rule возвращает правило комбинации масок оси.
func (c ScopeChain) Rule() ChainRule { return c.rule }

// combine объединяет маски ролей по правилу rule; маски идут от самого глубокого
// звена (ближайшего к субъекту) к корню, индекс 0 — самое глубокое.
// Возвращает пустую маску и false, когда все маски пусты.
func combine(rule ChainRule, masks []Roles) (Roles, bool) {
	switch rule {
	case Union:
		var sum Roles
		for _, m := range masks {
			sum.bits |= m.bits
		}
		return sum, sum.bits != 0
	default: // Nearest — подъём вверх к первой непустой маске
		for _, m := range masks {
			if m.bits != 0 {
				return m, true
			}
		}
		return RolesOf(0), false
	}
}

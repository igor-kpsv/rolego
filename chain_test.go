package rolego

import "testing"

// Тестовые типы звеньев для сборки оси.
const (
	kindA Kind = 1 + iota
	kindB
	kindC
)

func kindsEqual(got, want []Kind) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestScopeChainOrder(t *testing.T) {
	// Level сохраняются строго в порядке вызовов; последний тип — уровень родителя.
	c := NewScopeChain(Level(kindA), Level(kindB), Level(kindC))
	if got, want := c.Kinds(), []Kind{kindA, kindB, kindC}; !kindsEqual(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
}

func TestScopeChainRule(t *testing.T) {
	if got := NewScopeChain(Level(kindA), Combine(Union)).Rule(); got != Union {
		t.Errorf("Rule() = %v, want Union", got)
	}
	// Combine не вызван — правило по умолчанию Nearest (нулевое значение).
	if got := NewScopeChain(Level(kindA)).Rule(); got != Nearest {
		t.Errorf("Rule() = %v, want Nearest", got)
	}
}

func TestScopeChainInterleaved(t *testing.T) {
	// Combine между вызовами Level: порядок звеньев сохранён, правило применено.
	c := NewScopeChain(Level(kindA), Combine(Union), Level(kindB))
	if got, want := c.Kinds(), []Kind{kindA, kindB}; !kindsEqual(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
	if got := c.Rule(); got != Union {
		t.Errorf("Rule() = %v, want Union", got)
	}
}

func TestScopeChainLastCombineWins(t *testing.T) {
	// Повторный Combine: последний вызов выигрывает.
	c := NewScopeChain(Combine(Nearest), Combine(Union))
	if got := c.Rule(); got != Union {
		t.Errorf("Rule() = %v, want Union", got)
	}
}

func TestScopeChainKindsCopy(t *testing.T) {
	// Kinds() возвращает копию: мутация (включая append) возвращённого слайса
	// не меняет цепочку.
	c := NewScopeChain(Level(kindA), Level(kindB))
	got := c.Kinds()
	got[0] = kindC
	_ = append(got, kindC)

	if want := []Kind{kindA, kindB}; !kindsEqual(c.Kinds(), want) {
		t.Errorf("цепочка изменилась: Kinds() = %v, want %v", c.Kinds(), want)
	}
}

func TestScopeChainIgnoresUnknown(t *testing.T) {
	// Неизвестные типы опций игнорируются, а не паникуют.
	c := NewScopeChain("посторонний", 42, Level(kindB), nil, Combine(Union), struct{}{})
	if got, want := c.Kinds(), []Kind{kindB}; !kindsEqual(got, want) {
		t.Errorf("Kinds() = %v, want %v", got, want)
	}
	if c.Rule() != Union {
		t.Errorf("Rule() = %v, want Union", c.Rule())
	}
}

func TestScopeChainZeroValue(t *testing.T) {
	var c ScopeChain
	if got := c.Kinds(); len(got) != 0 {
		t.Errorf("нулевая ось: Kinds() = %v, want empty", got)
	}
	if c.Rule() != Nearest {
		t.Errorf("нулевая ось: Rule() = %v, want Nearest", c.Rule())
	}
}

func TestCombineNearest(t *testing.T) {
	// Индекс 0 — самое глубокое звено (ближайшее к субъекту); стоп на первой непустой.
	for _, tt := range []struct {
		name  string
		masks []Roles
		want  Roles
	}{
		{"самое глубокое непусто", []Roles{RolesOf(roleA), RolesOf(roleB)}, RolesOf(roleA)},
		{"глубокое пусто — берём родителя", []Roles{RolesOf(0), RolesOf(roleB), RolesOf(roleC)}, RolesOf(roleB)},
		{"пусто до корня — непустой корень", []Roles{RolesOf(0), RolesOf(0), RolesOf(roleC)}, RolesOf(roleC)},
		{"композитная маска", []Roles{RolesOf(roleA | roleC), RolesOf(roleB)}, RolesOf(roleA | roleC)},
	} {
		got, ok := combine(Nearest, tt.masks)
		if !ok {
			t.Errorf("%s: ok = false, want true", tt.name)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: combine = %#x, want %#x", tt.name, got.bits, tt.want.bits)
		}
	}
}

func TestCombineNearestAllEmpty(t *testing.T) {
	for _, tt := range []struct {
		name  string
		masks []Roles
	}{
		{"пустой слайс", nil},
		{"все маски пусты", []Roles{RolesOf(0), RolesOf(0)}},
	} {
		got, ok := combine(Nearest, tt.masks)
		if ok {
			t.Errorf("%s: ok = true, want false", tt.name)
			continue
		}
		if got != RolesOf(0) {
			t.Errorf("%s: combine = %#x, want RolesOf(0)", tt.name, got.bits)
		}
	}
}

func TestCombineUnion(t *testing.T) {
	for _, tt := range []struct {
		name  string
		masks []Roles
		want  Roles
	}{
		{"OR нескольких масок", []Roles{RolesOf(roleA), RolesOf(roleB | roleC), RolesOf(roleD)}, RolesOf(allRoles)},
		{"пересекающиеся маски", []Roles{RolesOf(roleA | roleB), RolesOf(roleB | roleC)}, RolesOf(roleA | roleB | roleC)},
		{"с пустыми масками", []Roles{RolesOf(0), RolesOf(roleA), RolesOf(0)}, RolesOf(roleA)},
		{"одна маска", []Roles{RolesOf(roleC | roleD)}, RolesOf(roleC | roleD)},
	} {
		got, ok := combine(Union, tt.masks)
		if !ok {
			t.Errorf("%s: ok = false, want true", tt.name)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: combine = %#x, want %#x", tt.name, got.bits, tt.want.bits)
		}
	}
}

func TestCombineUnionAllEmpty(t *testing.T) {
	for _, tt := range []struct {
		name  string
		masks []Roles
	}{
		{"пустой слайс", nil},
		{"все маски пусты", []Roles{RolesOf(0), RolesOf(0)}},
	} {
		got, ok := combine(Union, tt.masks)
		if ok {
			t.Errorf("%s: ok = true, want false", tt.name)
			continue
		}
		if got != RolesOf(0) {
			t.Errorf("%s: combine = %#x, want RolesOf(0)", tt.name, got.bits)
		}
	}
}

func TestCombineDoesNotMutate(t *testing.T) {
	masks := []Roles{RolesOf(0), RolesOf(roleB | roleC), RolesOf(roleA)}
	before := make([]Roles, len(masks))
	copy(before, masks)

	combine(Nearest, masks)
	combine(Union, masks)

	for i := range before {
		if masks[i] != before[i] {
			t.Errorf("masks[%d] изменён: %#x != %#x", i, masks[i].bits, before[i].bits)
		}
	}
}

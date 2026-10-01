package rolego

import "testing"

// Тестовые роли — четыре независимых бита — для составных масок.
const (
	roleA Role = 1 << iota
	roleB
	roleC
	roleD
)

const allRoles = roleA | roleB | roleC | roleD

// roleManager — роль-«менеджер», объявляемая только в иерархии (WithHierarchy):
// собственной записи в матрицах у неё нет, права она получает от предков.
// Отдельный бит за пределами roleA–roleD.
const roleManager Role = 1 << 4

func TestHas(t *testing.T) {
	subj := RolesOf(roleA | roleC)

	for _, tt := range []struct {
		name string
		role Role
		want bool
	}{
		{"одиночная роль в множестве", roleA, true},
		{"одиночной роли нет", roleB, false},
		{"составная маска с пересечением", roleA | roleB, true},
		{"составная маска без пересечения", roleB | roleD, false},
		{"маска всех ролей", allRoles, true},
	} {
		if got := subj.Has(tt.role); got != tt.want {
			t.Errorf("%s: Has(%#x) = %v, want %v", tt.name, tt.role, got, tt.want)
		}
	}
}

func TestHasAny(t *testing.T) {
	subj := RolesOf(roleA | roleC)

	for _, tt := range []struct {
		name string
		set  Role
		want bool
	}{
		{"пересечение одним битом", roleC | roleD, true},
		{"пересечения нет", roleB | roleD, false},
		{"одиночный бит из множества", roleA, true},
		{"нулевая маска", 0, false},
	} {
		if got := subj.HasAny(tt.set); got != tt.want {
			t.Errorf("%s: HasAny(%#x) = %v, want %v", tt.name, tt.set, got, tt.want)
		}
	}
}

func TestHasAll(t *testing.T) {
	subj := RolesOf(roleA | roleC)

	for _, tt := range []struct {
		name string
		set  Role
		want bool
	}{
		{"весь набор есть", roleA | roleC, true},
		{"один бит отсутствует", roleA | roleB, false},
		{"подмножество", roleC, true},
		{"запрошены все роли", allRoles, false},
	} {
		if got := subj.HasAll(tt.set); got != tt.want {
			t.Errorf("%s: HasAll(%#x) = %v, want %v", tt.name, tt.set, got, tt.want)
		}
	}
}

func TestEmptySet(t *testing.T) {
	empty := RolesOf(0)

	for _, role := range []Role{roleA, roleB, roleC, roleD, allRoles} {
		if empty.Has(role) {
			t.Errorf("пустое множество: Has(%#x) = true", role)
		}
		if empty.HasAny(role) {
			t.Errorf("пустое множество: HasAny(%#x) = true", role)
		}
		if empty.HasAll(role) {
			t.Errorf("пустое множество: HasAll(%#x) = true", role)
		}
	}

	if RolesOf(0) != (Roles{}) {
		t.Error("RolesOf(0) не равно нулевому значению Roles")
	}
}

func TestAdd(t *testing.T) {
	for _, tt := range []struct {
		name  string
		start Roles
		role  Role
		want  Roles
	}{
		{"в пустое множество", RolesOf(0), roleA, RolesOf(roleA)},
		{"новая роль", RolesOf(roleA), roleB, RolesOf(roleA | roleB)},
		{"уже присутствующая роль", RolesOf(roleA), roleA, RolesOf(roleA)},
		{"составная маска", RolesOf(roleA), roleB | roleC, RolesOf(roleA | roleB | roleC)},
	} {
		got := tt.start.Add(tt.role)
		if got != tt.want {
			t.Errorf("%s: Add(%#x) = %#x, want %#x", tt.name, tt.role, got.bits, tt.want.bits)
		}
	}
}

func TestAddIdempotent(t *testing.T) {
	base := RolesOf(roleA | roleB)
	once := base.Add(roleC)
	twice := once.Add(roleC)

	if once != twice {
		t.Errorf("двойное Add(%#x) изменило состояние: %#x != %#x", roleC, once.bits, twice.bits)
	}
	if base.Has(roleC) {
		t.Error("Add изменил исходное значение")
	}
}

func TestRemove(t *testing.T) {
	for _, tt := range []struct {
		name  string
		start Roles
		role  Role
		want  Roles
	}{
		{"существующая роль", RolesOf(roleA | roleB), roleA, RolesOf(roleB)},
		{"отсутствующая роль — no-op", RolesOf(roleA), roleB, RolesOf(roleA)},
		{"последняя роль", RolesOf(roleA), roleA, RolesOf(0)},
		{"составная маска", RolesOf(allRoles), roleA | roleB, RolesOf(roleC | roleD)},
	} {
		got := tt.start.Remove(tt.role)
		if got != tt.want {
			t.Errorf("%s: Remove(%#x) = %#x, want %#x", tt.name, tt.role, got.bits, tt.want.bits)
		}
	}
}

func TestRemoveIdempotent(t *testing.T) {
	base := RolesOf(allRoles)
	once := base.Remove(roleA)
	twice := once.Remove(roleA)

	if once != twice {
		t.Errorf("двойной Remove(%#x) изменил состояние: %#x != %#x", roleA, once.bits, twice.bits)
	}
	if !base.Has(roleA) {
		t.Error("Remove изменил исходное значение")
	}
}

func TestUnion(t *testing.T) {
	for _, tt := range []struct {
		name  string
		left  Roles
		right Roles
		want  Roles
	}{
		{"непересекающиеся множества", RolesOf(roleA | roleB), RolesOf(roleC | roleD), RolesOf(allRoles)},
		{"пересекающиеся множества", RolesOf(roleA | roleB), RolesOf(roleB | roleC), RolesOf(roleA | roleB | roleC)},
		{"объединение с пустым", RolesOf(roleA), RolesOf(0), RolesOf(roleA)},
	} {
		got := Roles{bits: tt.left.bits | tt.right.bits}
		if got != tt.want {
			t.Errorf("%s: объединение через | = %#x, want %#x", tt.name, got.bits, tt.want.bits)
		}
	}
}

func TestAll(t *testing.T) {
	if All != ^Perm(0) {
		t.Errorf("All = %#x, want ^Perm(0) = %#x", All, ^Perm(0))
	}
	if ^All != 0 {
		t.Errorf("All должен покрывать все 64 бита, не установлено: %#x", ^All)
	}
	for i := uint(0); i < 64; i++ {
		if All&(Perm(1)<<i) == 0 {
			t.Errorf("All не покрывает бит %d", i)
		}
	}
}

func TestDecisionAllow(t *testing.T) {
	if Deny == Allow {
		t.Fatal("Deny и Allow должны различаться")
	}
	for _, tt := range []struct {
		name string
		d    Decision
		want bool
	}{
		{"Deny", Deny, false},
		{"Allow", Allow, true},
	} {
		if got := tt.d.Allow(); got != tt.want {
			t.Errorf("%s.Allow() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

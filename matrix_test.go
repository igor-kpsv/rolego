package rolego

import "testing"

// Тестовые права: независимые биты.
const (
	right1 Perm = 1 << iota
	right2
	right3
)

// Тестовые типы ресурсов: свои Kind для проверки независимости матриц.
const (
	matrixKindA Kind = 100 + iota
	matrixKindB
)

func TestPerms(t *testing.T) {
	m := Matrix{
		roleA: right1,
		roleB: right2,
		roleD: right1 | right3,
	}

	for _, tt := range []struct {
		name  string
		roles Roles
		want  Perm
	}{
		{"одна роль в маске", RolesOf(roleA), right1},
		{"OR прав нескольких ролей", RolesOf(roleA | roleB), right1 | right2},
		{"составное право одной роли", RolesOf(roleD), right1 | right3},
		{"смесь записей и пропусков", RolesOf(roleB | roleD), right1 | right2 | right3},
		{"роль без записи — нулевой вклад", RolesOf(roleC), 0},
		{"пустая маска", RolesOf(0), 0},
		{"полная маска", RolesOf(allRoles), right1 | right2 | right3},
	} {
		if got := m.Perms(tt.roles); got != tt.want {
			t.Errorf("%s: Perms(%#x) = %#x, want %#x", tt.name, tt.roles.bits, got, tt.want)
		}
	}
}

func TestPermsNilMatrix(t *testing.T) {
	var m Matrix
	if got := m.Perms(RolesOf(roleA | roleB)); got != 0 {
		t.Errorf("Perms на nil-матрице = %#x, want 0", got)
	}
}

func TestAllow(t *testing.T) {
	m := Matrix{
		roleA: right1,
		roleB: right1 | right2,
	}

	for _, tt := range []struct {
		name  string
		roles Roles
		perm  Perm
		want  bool
	}{
		{"право выдано", RolesOf(roleA), right1, true},
		{"права нет ни у одной роли", RolesOf(roleA | roleB), right3, false},
		{"право у другой роли, у субъекта его нет", RolesOf(roleA), right2, false},
		{"пустая маска", RolesOf(0), right1, false},
	} {
		if got := m.Allow(tt.roles, tt.perm); got != tt.want {
			t.Errorf("%s: Allow(%#x, %#x) = %v, want %v", tt.name, tt.roles.bits, tt.perm, got, tt.want)
		}
	}
}

func TestAtLeastOneRole(t *testing.T) {
	// Право выдано двум ролям, но у субъекта только одна: контракт «хотя бы у
	// одной роли есть право» — достаточно одного носителя из маски.
	m := Matrix{
		roleA: right1 | right2,
		roleB: right2,
	}

	if got := m.Allow(RolesOf(roleA), right1); !got {
		t.Error("Allow(right1) = false, want true: право выдано roleA")
	}
	if got := m.Perms(RolesOf(roleA)); got != right1|right2 {
		t.Errorf("Perms(roleA) = %#x, want %#x", got, right1|right2)
	}
	// right2 выдан roleA и roleB; у субъекта только один носитель.
	if got := m.Allow(RolesOf(roleB), right2); !got {
		t.Error("Allow(right2) = false, want true: право выдано двум ролям, одной достаточно")
	}
}

func TestMatricesNoCrossKind(t *testing.T) {
	// Один и тот же бит права на разных Kind не смешивается: kindA хранит
	// права roleA, kindB — roleC, и каждый должен видеть только своё.
	mats := Matrices{
		matrixKindA: {roleA: right1},
		matrixKindB: {roleC: right1},
	}

	if got := mats[matrixKindA].Perms(RolesOf(roleA | roleC)); got != right1 {
		t.Errorf("kindA: Perms = %#x, want %#x (право roleC из kindB не должно попасть)", got, right1)
	}
	if got := mats[matrixKindB].Perms(RolesOf(roleA | roleC)); got != right1 {
		t.Errorf("kindB: Perms = %#x, want %#x (право roleA из kindA не должно попасть)", got, right1)
	}
}

func TestMatricesSameRoleDifferentRights(t *testing.T) {
	// Два Kind выдают одной и той же роли разные права: проверка каждого
	// Kind даёт только свои записи, без пересечения.
	mats := Matrices{
		matrixKindA: {roleA: right1},
		matrixKindB: {roleA: right2},
	}

	if got := mats[matrixKindA].Perms(RolesOf(roleA)); got != right1 {
		t.Errorf("kindA: Perms(roleA) = %#x, want %#x", got, right1)
	}
	if got := mats[matrixKindA].Allow(RolesOf(roleA), right2); got {
		t.Error("kindA: Allow(right2) = true, want false (право принадлежит kindB)")
	}
	if got := mats[matrixKindB].Perms(RolesOf(roleA)); got != right2 {
		t.Errorf("kindB: Perms(roleA) = %#x, want %#x", got, right2)
	}
	if got := mats[matrixKindB].Allow(RolesOf(roleA), right1); got {
		t.Error("kindB: Allow(right1) = true, want false (право принадлежит kindA)")
	}
}

package rolego

import (
	"context"
	"errors"
	"testing"
)

// roleC включает roleB, roleB включает roleA: права «наследуются вверх».
var testHierarchy = Hierarchy{
	roleB: roleA,
	roleC: roleB,
}

// newRoleCheckerFor собирает Checker с заданной иерархией и матрицей.
// Ось — одно звено KindDoor, резолвер отдаёт держателю роль holder.
func newRoleCheckerFor(mats Matrices, holder Role, h Hierarchy) (*Checker[string, testResource], error) {
	return New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindDoor: holder}),
		WithPolicy[string, testResource](mats, NewScopeChain(Level(KindDoor))),
		WithHierarchy[string, testResource](h),
	)
}

// newRoleChecker собирает Checker с иерархией testHierarchy и данной матрицей.
func newRoleChecker(mats Matrices, holder Role) (*Checker[string, testResource], error) {
	return newRoleCheckerFor(mats, holder, testHierarchy)
}

func TestWithHierarchyTransitive(t *testing.T) {
	// Матрица выдаёт right1 только roleA, right2 только roleC. Через иерархию
	// держатель roleC получает оба права, держатель roleA — только своё.
	mats := Matrices{KindDoor: {roleA: right1, roleC: right2}}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}

	for _, tt := range []struct {
		name   string
		holder Role
		perm   Perm
		want   Decision
	}{
		{"вершина наследует право внука", roleC, right1, Allow},
		{"вершина сохраняет своё право", roleC, right2, Allow},
		{"промежуточная роль наследует право предка", roleB, right1, Allow},
		{"промежуточная роль не получает право потомка", roleB, right2, Deny},
		{"базовая роль не получает чужих прав", roleA, right2, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newRoleChecker(mats, tt.holder)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := c.Check(context.Background(), "alice", res, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestWithHierarchyCycle(t *testing.T) {
	// Взаимное включение — цикл: New возвращает ошибку, сверяем errors.Is.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindDoor: roleA}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1}},
			NewScopeChain(Level(KindDoor)),
		),
		WithHierarchy[string, testResource](Hierarchy{roleA: roleB, roleB: roleA}),
	)
	if c != nil {
		t.Errorf("New() = %v, want nil при цикле в иерархии", c)
	}
	if !errors.Is(err, ErrHierarchyCycle) {
		t.Errorf("New() error = %v, want errors.Is(ErrHierarchyCycle)", err)
	}
}

func TestWithHierarchyDoesNotMutateInput(t *testing.T) {
	mats := Matrices{KindDoor: {roleC: right2}}
	if _, err := newRoleChecker(mats, roleC); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := mats[KindDoor][roleC]; got != right2 {
		t.Errorf("исходная матрица роль roleC = %#x, want %#x (карта не должна мутироваться)",
			got, right2)
	}
}

func TestWithHierarchyOrderIndependent(t *testing.T) {
	// WithHierarchy до WithPolicy — тот же результат: расширение выполняется
	// после всех опций в New.
	c, err := New[string, testResource](
		WithHierarchy[string, testResource](testHierarchy),
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindDoor: roleC}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := c.Check(context.Background(), "alice",
		testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}, right1)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got != Allow {
		t.Errorf("Check(right1) = %v, want Allow (наследование не зависит от порядка опций)", got)
	}
}

func TestWithHierarchyEmptyIsNoOp(t *testing.T) {
	// Пустая иерархия — поведение как без неё: чужое право не наследуется.
	for _, h := range []Hierarchy{nil, {}} {
		c, err := New[string, testResource](
			Type[string, testResource](KindDoor),
			MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
			Resolve[string, testResource](kindResolver{KindDoor: roleB}),
			WithPolicy[string, testResource](
				Matrices{KindDoor: {roleA: right1}},
				NewScopeChain(Level(KindDoor)),
			),
			WithHierarchy[string, testResource](h),
		)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		got, _ := c.Check(context.Background(), "alice",
			testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}, right1)
		if got != Deny {
			t.Errorf("Check(right1) с пустой иерархией = %v, want Deny (право roleB не выдано)", got)
		}
	}
}

func TestWithHierarchyRoleDeclaredOnly(t *testing.T) {
	// Роль-«менеджер», объявленная только в иерархии и отсутствующая в матрице,
	// получает права предков по цепочке, но не права несвязанных ролей.
	// Матрица — «зритель и редактор»: roleA даёт right1, roleC даёт right2.
	type check struct {
		perm Perm
		want Decision
	}
	mats := Matrices{KindDoor: {roleA: right1, roleC: right2}}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}

	for _, tt := range []struct {
		name      string
		hierarchy Hierarchy
		checks    []check
	}{
		{
			// Менеджер — ребёнок редактора: наследует и right1 зрителя-предка,
			// и right2 прямого родителя.
			name:      "менеджер под редактором",
			hierarchy: Hierarchy{roleB: roleA, roleC: roleB, roleManager: roleC},
			checks: []check{
				{right1, Allow},
				{right2, Allow},
				{right3, Deny}, // нигде не выписано — чужое право
			},
		},
		{
			// Менеджер — ребёнок промежуточной роли: right1 предка есть,
			// right2 редактора — не предок, значит Deny.
			name:      "менеджер под промежуточной ролью",
			hierarchy: Hierarchy{roleB: roleA, roleC: roleB, roleManager: roleB},
			checks: []check{
				{right1, Allow},
				{right2, Deny},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newRoleCheckerFor(mats, roleManager, tt.hierarchy)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			for _, ck := range tt.checks {
				got, err := c.Check(context.Background(), "alice", res, ck.perm)
				if err != nil {
					t.Fatalf("Check(%#x) error = %v", ck.perm, err)
				}
				if got != ck.want {
					t.Errorf("Check(%#x) = %v, want %v", ck.perm, got, ck.want)
				}
			}
		})
	}
}

func TestWithHierarchyComposedParent(t *testing.T) {
	// Составная маска родителей: roleC наследует roleA|roleB сразу — оба права и
	// транзитивные предки; roleB сохраняет своё право и наследует roleA.
	mats := Matrices{KindDoor: {roleA: right1, roleB: right2}}
	h := Hierarchy{roleB: roleA, roleC: roleA | roleB}

	for _, tt := range []struct {
		name   string
		holder Role
		perm   Perm
		want   Decision
	}{
		{"ребёнок наследует право первого родителя", roleC, right1, Allow},
		{"ребёнок наследует право второго родителя", roleC, right2, Allow},
		{"ребёнок получает составное право", roleC, right1 | right2, Allow},
		{"ребёнок не получает чужого права", roleC, right3, Deny},
		{"промежуточная роль наследует предка", roleB, right1, Allow},
		{"промежуточная роль сохраняет своё право", roleB, right2, Allow},
		{"базовая роль не наследует от детей", roleA, right2, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newRoleCheckerFor(mats, tt.holder, h)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := c.Check(context.Background(), "alice",
				testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestWithHierarchyDiamond(t *testing.T) {
	// Ромб: общий предок roleA достижим двумя ветками (roleB и roleC); roleD с
	// двумя родителями собирает права обеих веток без ложного детекта цикла,
	// рекурсия сходится, права общего предка складываются по OR.
	mats := Matrices{KindDoor: {roleA: right1, roleD: right2}}
	h := Hierarchy{roleB: roleA, roleC: roleA, roleD: roleB | roleC}

	for _, tt := range []struct {
		name   string
		holder Role
		perm   Perm
		want   Decision
	}{
		{"роль ромба наследует право общего предка", roleD, right1, Allow},
		{"роль ромба сохраняет своё право", roleD, right2, Allow},
		{"роль ромба получает составное право", roleD, right1 | right2, Allow},
		{"роль ромба не получает чужого права", roleD, right3, Deny},
		{"ветка roleB наследует предка", roleB, right1, Allow},
		{"ветка roleB не получает право другой ветки", roleB, right2, Deny},
		{"ветка roleC наследует предка", roleC, right1, Allow},
		{"ветка roleC не получает право другой ветки", roleC, right2, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := newRoleCheckerFor(mats, tt.holder, h)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := c.Check(context.Background(), "alice",
				testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestWithHierarchyCycleSelfAndChain(t *testing.T) {
	// Самопетля и цикл длины 3: New возвращает (nil, ErrHierarchyCycle).
	for _, tt := range []struct {
		name      string
		hierarchy Hierarchy
	}{
		{"самопетля", Hierarchy{roleA: roleA}},
		{"цикл длины 3", Hierarchy{roleA: roleB, roleB: roleC, roleC: roleA}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New[string, testResource](
				Type[string, testResource](KindDoor),
				MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
				Resolve[string, testResource](kindResolver{KindDoor: roleA}),
				WithPolicy[string, testResource](
					Matrices{KindDoor: {roleA: right1}},
					NewScopeChain(Level(KindDoor)),
				),
				WithHierarchy[string, testResource](tt.hierarchy),
			)
			if c != nil {
				t.Errorf("New() = %v, want nil при цикле в иерархии", c)
			}
			if !errors.Is(err, ErrHierarchyCycle) {
				t.Errorf("New() error = %v, want errors.Is(ErrHierarchyCycle)", err)
			}
		})
	}
}

func TestWithHierarchyChildAndParentSameMask(t *testing.T) {
	// Резолвер выдаёт маску roleB|roleC — ребёнка и родителя сразу: право предка
	// даёт Allow (OR идемпотентен, вклад ролей не задваивается), не выписанные
	// права не появляются.
	mats := Matrices{KindDoor: {roleA: right1}}
	c, err := newRoleChecker(mats, roleB|roleC)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}

	for _, tt := range []struct {
		name string
		perm Perm
		want Decision
	}{
		{"право предка — Allow", right1, Allow},
		{"комбинация с непокрытым правом — Allow", right1 | right3, Allow},
		{"не выписанное право — Deny", right3, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Check(context.Background(), "alice", res, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}
}

func TestWithHierarchyPerKindIndependent(t *testing.T) {
	// Иерархия разворачивается в каждую матрицу вида отдельно: одна роль-наследник
	// получает право предка на своём виде, а право, выписанное только в матрице
	// другого вида, в решение не протекает.
	mats := Matrices{
		KindDoor: {roleA: right1},
		KindRoom: {roleA: right1, roleC: right2},
	}
	h := Hierarchy{roleB: roleA, roleC: roleB}
	res := testResource{scopes: []Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}}

	for _, tt := range []struct {
		name string
		kind Kind
		perm Perm
		want Decision
	}{
		{"дверь: роль-наследник в своей матрице", KindDoor, right1, Allow},
		{"дверь: право из матрицы комнаты не протекает", KindDoor, right2, Deny},
		{"комната: роль-наследник в своей матрице", KindRoom, right1, Allow},
		{"комната: собственное право роли в своей матрице", KindRoom, right2, Allow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New[string, testResource](
				Type[string, testResource](tt.kind),
				MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
				Resolve[string, testResource](kindResolver{KindDoor: roleC, KindRoom: roleC}),
				WithPolicy[string, testResource](
					mats,
					NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Union)),
				),
				WithHierarchy[string, testResource](h),
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := c.Check(context.Background(), "alice", res, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) на %v = %v, want %v", tt.perm, tt.kind, got, tt.want)
			}
		})
	}
}

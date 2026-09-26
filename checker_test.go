package rolego

import (
	"context"
	"errors"
	"testing"
)

// Тестовые типы ресурсов для сценария «ключ — дверь».
const (
	KindDoor Kind = 200 + iota
	KindRoom
)

// Роли сценария: держатель ключа и производная «авторская» роль.
const (
	RoleKeyholder Role = 1 << 10
	RoleAuthor    Role = 1 << 11
)

// Право сценария «ключ — дверь».
const OpenDoor Perm = 1 << 5

// testResource — ресурс теста: имя субъекта-владельца и звенья ресурса.
type testResource struct {
	owner  string
	scopes []Scope
}

// doorResolver — резолвер «держатель ключа из данных»: маску ключника даёт
// только на звене двери, и только перечисленным субъектам.
type doorResolver map[string]Role

func (r doorResolver) RolesAt(_ context.Context, subj string, link Link) (Roles, error) {
	if link.Kind != KindDoor {
		return RolesOf(0), nil
	}
	return RolesOf(r[subj]), nil
}

// noneResolver — резолвер, не выдающий ни одной роли ни на одном звене.
type noneResolver struct{}

func (noneResolver) RolesAt(context.Context, string, Link) (Roles, error) { return RolesOf(0), nil }

// kindResolver — резолвер «роль по типу звена из справочника».
type kindResolver map[Kind]Role

func (r kindResolver) RolesAt(_ context.Context, _ string, link Link) (Roles, error) {
	return RolesOf(r[link.Kind]), nil
}

// recordingResolver — резолвер-регистратор: запоминает переданные звенья и на
// звене типа KindDoor выдаёт производную роль RoleAuthor.
type recordingResolver struct {
	links []Link
}

func (r *recordingResolver) RolesAt(_ context.Context, _ string, link Link) (Roles, error) {
	r.links = append(r.links, link)
	if link.Kind == KindDoor {
		return RolesOf(RoleAuthor), nil
	}
	return RolesOf(0), nil
}

// errAtResolver — резолвер с заданной ошибкой на звене типа linkErrKind; на
// остальных звеньях выдаёт роль roleA.
type errAtResolver struct {
	linkErrKind Kind
	err         error
}

func (r errAtResolver) RolesAt(_ context.Context, _ string, link Link) (Roles, error) {
	if link.Kind == r.linkErrKind {
		return RolesOf(0), r.err
	}
	return RolesOf(roleA), nil
}

// Компиляционные проверки: все резолверы реализуют интерфейс Resolver.
var (
	_ Resolver[string, testResource] = doorResolver{}
	_ Resolver[string, testResource] = noneResolver{}
	_ Resolver[string, testResource] = kindResolver{}
	_ Resolver[string, testResource] = (*recordingResolver)(nil)
	_ Resolver[string, testResource] = errAtResolver{}
)

// newDoorChecker собирает Checker сценария «ключ — дверь»: ось из одного звена
// KindDoor, правило Nearest (по умолчанию), ключник из данных; матрица выдаёт
// OpenDoor только держателю ключа.
func newDoorChecker() (*Checker[string, testResource], error) {
	return New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](doorResolver{"alice": RoleKeyholder}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
}

func TestCheckKeyholder(t *testing.T) {
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{owner: "alice", scopes: []Scope{{Kind: KindDoor, ID: 7}}}

	for _, tt := range []struct {
		name string
		subj string
		want Decision
	}{
		{"держатель ключа — Allow", "alice", Allow},
		{"не держатель — Deny", "bob", Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Check(context.Background(), tt.subj, res, OpenDoor)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%q) = %v, want %v", tt.subj, got, tt.want)
			}
		})
	}

	t.Run("ключник без права в матрице — Deny", func(t *testing.T) {
		got, err := c.Check(context.Background(), "alice", res, right2)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got != Deny {
			t.Errorf("Check(right2) = %v, want Deny (в матрице только OpenDoor)", got)
		}
	})
}

func TestCheckDerivedRole(t *testing.T) {
	// Резолвер добавляет производную роль по типу звена (аналог Author в
	// концепте); проверяем и решение, и что резолвер получает корректный Link.
	rec := &recordingResolver{}
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](rec),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleAuthor: right2}},
			NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Nearest)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindRoom, ID: 3}, {Kind: KindDoor, ID: 9}}}

	got, err := c.Check(context.Background(), "draft", res, right2)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got != Allow {
		t.Errorf("Check(производная роль) = %v, want Allow", got)
	}

	// Резолвер получил звенья от самого глубокого (дверь, depth 0) к корню
	// (комната, depth 1): Link несёт копию Scope и Kind звена.
	want := []Link{
		{Scope: Scope{Kind: KindDoor, ID: 9}, Kind: KindDoor, Depth: 0},
		{Scope: Scope{Kind: KindRoom, ID: 3}, Kind: KindRoom, Depth: 1},
	}
	if len(rec.links) != len(want) {
		t.Fatalf("RolesAt вызван %d раз, want %d", len(rec.links), len(want))
	}
	for i := range want {
		if rec.links[i] != want[i] {
			t.Errorf("RolesAt[%d] = %+v, want %+v", i, rec.links[i], want[i])
		}
	}

	// Производная роль покрывает только право, выписанное в матрице.
	if got, _ := c.Check(context.Background(), "draft", res, right1); got != Deny {
		t.Errorf("Check(right1) = %v, want Deny (автор без right1)", got)
	}
}

func TestCheckResolverError(t *testing.T) {
	for _, tt := range []struct {
		name        string
		linkErrKind Kind
		scopes      []Scope
	}{
		{"ошибка на самом глубоком звене", KindDoor,
			[]Scope{{Kind: KindDoor, ID: 1}}},
		{"ошибка на корневом звене", KindRoom,
			[]Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := New[string, testResource](
				Type[string, testResource](KindDoor),
				MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
				Resolve[string, testResource](errAtResolver{linkErrKind: tt.linkErrKind, err: mockErr}),
				WithPolicy[string, testResource](
					Matrices{KindDoor: {roleA: right1}},
					NewScopeChain(Level(KindDoor), Level(KindRoom)),
				),
			)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			got, err := c.Check(context.Background(), "alice", testResource{scopes: tt.scopes}, right1)
			if got != Deny {
				t.Errorf("Check() = %v, want Deny при ошибке резолвера", got)
			}
			if !errors.Is(err, mockErr) {
				t.Errorf("Check() error = %v, want %v (сверка errors.Is)", err, mockErr)
			}
		})
	}
}

func TestCheckNoRoles(t *testing.T) {
	// Ни на одном звене ролей нет — Deny без ошибки, даже если право выписано.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](noneResolver{}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1}},
			NewScopeChain(Level(KindDoor), Level(KindRoom)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := c.Check(context.Background(), "bob",
		testResource{scopes: []Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}}, right1)
	if got != Deny {
		t.Errorf("Check() = %v, want Deny (ролей нет ни на одном звене)", got)
	}
	if err != nil {
		t.Errorf("Check() error = %v, want nil", err)
	}
}

func TestCheckNoScopes(t *testing.T) {
	// У ресурса нет звеньев (nil от MapScopes) — Deny без паники; при этом
	// экстрактор вызывается ровно один раз.
	calls := 0
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope {
			calls++
			return r.scopes
		}),
		Resolve[string, testResource](doorResolver{"alice": RoleKeyholder}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := c.Check(context.Background(), "alice", testResource{}, OpenDoor)
	if got != Deny {
		t.Errorf("Check() без звеньев = %v, want Deny", got)
	}
	if err != nil {
		t.Errorf("Check() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("MapScopes вызван %d раз, want ровно 1", calls)
	}
}

func TestCheckMapScopesOnce(t *testing.T) {
	// Успешный проход тоже вызывает MapScopes ровно один раз.
	calls := 0
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope {
			calls++
			return []Scope{{Kind: KindDoor, ID: 1}}
		}),
		Resolve[string, testResource](doorResolver{"alice": RoleKeyholder}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := c.Check(context.Background(), "alice", testResource{}, OpenDoor); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("MapScopes вызван %d раз, want ровно 1", calls)
	}
}

func TestCheckNearestClimb(t *testing.T) {
	// У двери (глубокое звено) ролей нет, у корня (комната) роль есть — правило
	// Nearest поднимается от ресурса к корню и разрешает.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindRoom: roleC}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleC: OpenDoor}},
			NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Nearest)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}}
	got, err := c.Check(context.Background(), "bob", res, OpenDoor)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got != Allow {
		t.Errorf("Check() = %v, want Allow (Nearest поднял роль корня)", got)
	}
}

func TestCheckUnion(t *testing.T) {
	// Union — OR ролей всех звеньев: право, выписанное роли у корня, срабатывает
	// на ресурсе, даже если у самого ресурса этой роли нет.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindDoor: roleA, KindRoom: roleB}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1, roleB: right2}},
			NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Union)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}}

	for _, tt := range []struct {
		name string
		perm Perm
		want Decision
	}{
		{"право корневой роли — Allow", right2, Allow},
		{"право роли ресурса — Allow", right1, Allow},
		{"составное право — Allow", right1 | right2, Allow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Check(context.Background(), "bob", res, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v", tt.perm, got, tt.want)
			}
		})
	}

	t.Run("Nearest не дотягивает роль корня", func(t *testing.T) {
		// Контраст: с Nearest маска только двери (roleA) — право roleB недоступно.
		cn, err := New[string, testResource](
			Type[string, testResource](KindDoor),
			MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
			Resolve[string, testResource](kindResolver{KindDoor: roleA, KindRoom: roleB}),
			WithPolicy[string, testResource](
				Matrices{KindDoor: {roleA: right1, roleB: right2}},
				NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Nearest)),
			),
		)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		got, err := cn.Check(context.Background(), "bob", res, right2)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got != Deny {
			t.Errorf("Check(right2) = %v, want Deny (Nearest взял только маску двери)", got)
		}
	})
}

func TestCheckNoMatrixForKind(t *testing.T) {
	// В Matrices нет матрицы для Kind из Type — безопасный Deny без ошибки
	// (полнота матриц проверяется на Этапе 6 в Validate).
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope {
			return []Scope{{Kind: KindDoor, ID: 1}}
		}),
		Resolve[string, testResource](kindResolver{KindDoor: roleA}),
		WithPolicy[string, testResource](
			Matrices{},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := c.Check(context.Background(), "alice", testResource{}, right1)
	if got != Deny {
		t.Errorf("Check() = %v, want Deny (матрицы для KindDoor нет)", got)
	}
	if err != nil {
		t.Errorf("Check() error = %v, want nil", err)
	}
}

func TestCheckWithPolicyLastWins(t *testing.T) {
	// Повторный WithPolicy — последний побеждает: вторая политика перезаписывает
	// и матрицу, и правило оси (Nearest вместо забытой Union из первой).
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](kindResolver{KindDoor: roleA, KindRoom: roleB}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1, roleB: right1}},
			NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Union)),
		),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1, roleB: right2}},
			NewScopeChain(Level(KindDoor), Level(KindRoom), Combine(Nearest)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindRoom, ID: 1}, {Kind: KindDoor, ID: 2}}}

	t.Run("матрица из последней политики", func(t *testing.T) {
		got, err := c.Check(context.Background(), "bob", res, right1)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got != Allow {
			t.Errorf("Check(right1) = %v, want Allow (roleA из второй матрицы)", got)
		}
	})

	t.Run("правило оси из последней политики", func(t *testing.T) {
		// Nearest видит только маску двери (roleA): право второй матрицы у
		// корня (roleB → right2) недостижимо; Union из первой была перезаписана.
		got, err := c.Check(context.Background(), "bob", res, right2)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if got != Deny {
			t.Errorf("Check(right2) = %v, want Deny (Nearest из второй политики)", got)
		}
	})
}

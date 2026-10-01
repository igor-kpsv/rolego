package rolego

import (
	"context"
	"errors"
	"testing"
)

func TestRolesForNearest(t *testing.T) {
	// У самого ресурса ролей нет, у корня — roleC: Nearest поднимает маску корня,
	// и RolesFor отдаёт её без участия матрицы прав.
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

	roles, err := c.RolesFor(context.Background(), "bob", res)
	if err != nil {
		t.Fatalf("RolesFor() error = %v", err)
	}
	if !roles.Has(roleC) {
		t.Errorf("RolesFor() = %#x, want роль roleC (Nearest поднял маску корня)", roles.bits)
	}

	// Тот же конвейер, что в Check: право roleC на ресурсе проходит.
	if dec, _ := c.Check(context.Background(), "bob", res, OpenDoor); !dec.Allow() {
		t.Errorf("Check(OpenDoor) не Allow, хотя RolesFor отдаёт roleC")
	}
}

func TestRolesForUnion(t *testing.T) {
	// Union — OR ролей всех звеньев: маска содержит обе роли целиком.
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

	roles, err := c.RolesFor(context.Background(), "bob", res)
	if err != nil {
		t.Fatalf("RolesFor() error = %v", err)
	}
	if !roles.HasAll(roleA | roleB) {
		t.Errorf("RolesFor() = %#x, want обе роли (Union OR всех звеньев)", roles.bits)
	}
}

func TestRolesForNoScopes(t *testing.T) {
	// Нет звеньев у ресурса — пустое множество ролей без ошибки.
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	roles, err := c.RolesFor(context.Background(), "alice", testResource{})
	if err != nil {
		t.Fatalf("RolesFor() error = %v", err)
	}
	if roles.bits != 0 {
		t.Errorf("RolesFor() без звеньев = %#x, want 0", roles.bits)
	}
}

func TestRolesForResolverError(t *testing.T) {
	// Ошибка резолвера пробрасывается как (RolesOf(0), err) — сверка errors.Is.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](errAtResolver{linkErrKind: KindDoor, err: errMock}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	roles, err := c.RolesFor(context.Background(), "alice",
		testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}})
	if roles.bits != 0 {
		t.Errorf("RolesFor() при ошибке = %#x, want 0", roles.bits)
	}
	if !errors.Is(err, errMock) {
		t.Errorf("RolesFor() error = %v, want errors.Is(errMock)", err)
	}
}

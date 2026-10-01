package rolego

import (
	"context"
	"fmt"
)

// Сценарий README «ключ — дверь»: роль — держатель ключа (RoleKeyholder), право —
// открыть дверь (OpenDoor), резолвер — карта «субъект → роль» из твоих данных,
// ось — одно звено KindDoor, матрица выдаёт OpenDoor только ключнику.

// doorResource — дверь примера: звенья ресурса извлекает свой MapScopes.
type doorResource struct {
	scopes []Scope
}

// holderResolver — резолвер примера: роль выдаёт только на звене двери, и только
// субъектам из карты (кто держатель ключа — твои данные).
type holderResolver map[string]Role

func (r holderResolver) RolesAt(_ context.Context, subj string, link Link) (Roles, error) {
	if link.Kind != KindDoor {
		return RolesOf(0), nil
	}
	return RolesOf(r[subj]), nil
}

func ExampleChecker_Check() {
	c, err := New[string, doorResource](
		Type[string, doorResource](KindDoor),
		MapScopes[string, doorResource](func(d doorResource) []Scope { return d.scopes }),
		Resolve[string, doorResource](holderResolver{"alice": RoleKeyholder}),
		WithPolicy[string, doorResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}
	door := doorResource{scopes: []Scope{{Kind: KindDoor, ID: 7}}}

	dec, err := c.Check(context.Background(), "alice", door, OpenDoor) // у алисы ключ
	if err != nil {
		panic(err)
	}
	fmt.Println(dec.Allow())

	dec, err = c.Check(context.Background(), "bob", door, OpenDoor) // у боба ключа нет
	if err != nil {
		panic(err)
	}
	fmt.Println(dec.Allow())

	// Output:
	// true
	// false
}

func ExampleChecker_RolesFor() {
	c, err := New[string, doorResource](
		Type[string, doorResource](KindDoor),
		MapScopes[string, doorResource](func(d doorResource) []Scope { return d.scopes }),
		Resolve[string, doorResource](holderResolver{"alice": RoleKeyholder}),
		WithPolicy[string, doorResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}
	door := doorResource{scopes: []Scope{{Kind: KindDoor, ID: 7}}}

	// «Какие роли у алисы на этой двери?» — без проверки конкретного права.
	roles, err := c.RolesFor(context.Background(), "alice", door)
	if err != nil {
		panic(err)
	}
	fmt.Println(roles.Has(RoleKeyholder))

	// Output:
	// true
}

func ExampleRegistry() {
	const (
		Read Perm = 1 << iota
		Write
		Delete
	)

	// Строковые права из унаследованной БД: «documents.view» и т.п.
	reg := new(Registry[Perm])
	reg.Register("documents.view", Read)
	reg.Register("documents.edit", Read|Write)
	reg.Register("documents.delete", Delete)

	v, err := reg.Parse("documents.edit")
	if err != nil {
		panic(err)
	}
	fmt.Println(v == Read|Write)

	fmt.Println(reg.MustParse("documents.view") == Read)

	name, ok := reg.Name(Delete)
	fmt.Println(name, ok)

	// Output:
	// true
	// true
	// documents.delete true
}

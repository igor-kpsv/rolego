package rolego

import (
	"context"
	"errors"
	"testing"
)

func TestValidateOK(t *testing.T) {
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestValidateEmptyChain(t *testing.T) {
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](doorResolver{}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrEmptyChain) {
		t.Errorf("Validate() = %v, want errors.Is(ErrEmptyChain)", err)
	}
}

func TestValidateNoMatrixForKind(t *testing.T) {
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](doorResolver{}),
		WithPolicy[string, testResource](
			Matrices{},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrNoMatrixForKind) {
		t.Errorf("Validate() = %v, want errors.Is(ErrNoMatrixForKind)", err)
	}
}

func TestValidateNilResolver(t *testing.T) {
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](nil),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrNilResolver) {
		t.Errorf("Validate() = %v, want errors.Is(ErrNilResolver)", err)
	}
}

func TestValidateTypedNilResolver(t *testing.T) {
	// Типизированный nil-указатель с методом RolesAt: интерфейс формально не nil,
	// но вызов метода упадёт (метод разыменовывает приёмник) — Validate должен
	// поймать его как nil.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource]((*recordingResolver)(nil)),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrNilResolver) {
		t.Errorf("Validate() = %v, want errors.Is(ErrNilResolver)", err)
	}
}

func TestValidateNilMapScopes(t *testing.T) {
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](nil),
		Resolve[string, testResource](doorResolver{}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrNilMapScopes) {
		t.Errorf("Validate() = %v, want errors.Is(ErrNilMapScopes)", err)
	}
}

func TestValidateInvalidChainRule(t *testing.T) {
	// Правило комбинации не равно Nearest/Union: Validate ловит его на старте,
	// а не в момент комбинации.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](doorResolver{}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor), Combine(ChainRule(42))),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := c.Validate(); !errors.Is(err, ErrInvalidChainRule) {
		t.Errorf("Validate() = %v, want errors.Is(ErrInvalidChainRule)", err)
	}
}

func TestValidateAccumulates(t *testing.T) {
	// Несколько противоречий сразу: errors.Join содержит все, каждое ловится через
	// errors.Is.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](nil),
		Resolve[string, testResource](nil),
		WithPolicy[string, testResource](
			Matrices{},
			NewScopeChain(),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	err = c.Validate()
	for _, want := range []error{ErrEmptyChain, ErrNoMatrixForKind, ErrNilResolver, ErrNilMapScopes} {
		if !errors.Is(err, want) {
			t.Errorf("Validate() = %v, не содержит %v", err, want)
		}
	}
}

func TestWhoCanFilters(t *testing.T) {
	// Субъект-детерминированная политика: у кого в карте резолвера есть роль —
	// тот в результате; порядок как в candidates, не-держатели отсеиваются.
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](doorResolver{
			"alice": RoleKeyholder,
			"carol": RoleKeyholder,
		}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {RoleKeyholder: OpenDoor}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}
	candidates := []string{"xavier", "alice", "bob", "carol", "bob"}

	got, err := c.WhoCan(context.Background(), candidates, res, OpenDoor)
	if err != nil {
		t.Fatalf("WhoCan() error = %v", err)
	}
	want := []string{"alice", "carol"}
	if len(got) != len(want) {
		t.Fatalf("WhoCan() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("WhoCan()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestWhoCanNoCandidates(t *testing.T) {
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	got, err := c.WhoCan(context.Background(), nil,
		testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}, OpenDoor)
	if err != nil {
		t.Fatalf("WhoCan() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("WhoCan() = %v, want пустой результат", got)
	}
}

func TestWhoCanResolverError(t *testing.T) {
	// Ошибка резолвера на первом же кандидате: (nil, err) без частичного результата.
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
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}
	got, err := c.WhoCan(context.Background(), []string{"alice", "bob", "carol"}, res, right1)
	if got != nil {
		t.Errorf("WhoCan() = %v, want nil при ошибке резолвера", got)
	}
	if !errors.Is(err, errMock) {
		t.Errorf("WhoCan() error = %v, want errors.Is(errMock)", err)
	}
}

func TestCheckZeroPerm(t *testing.T) {
	// Нулевое право — бессмысленный запрос: Deny + ErrZeroPerm ещё до политики,
	// даже для субъекта с ключом.
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}
	dec, err := c.Check(context.Background(), "alice", res, 0)
	if dec != Deny {
		t.Errorf("Check(perm=0) = %v, want Deny", dec)
	}
	if !errors.Is(err, ErrZeroPerm) {
		t.Errorf("Check(perm=0) error = %v, want errors.Is(ErrZeroPerm)", err)
	}
}

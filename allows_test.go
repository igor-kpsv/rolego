package rolego

import (
	"context"
	"testing"
)

// TestAllows — bool-обёртка: Allow возвращает true, Deny — false.
func TestAllows(t *testing.T) {
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("newDoorChecker() error = %v", err)
	}
	res := testResource{owner: "alice", scopes: []Scope{{Kind: KindDoor, ID: 7}}}

	for _, tt := range []struct {
		name string
		subj string
		want bool
	}{
		{"держатель ключа — Allow", "alice", true},
		{"не держатель — Deny", "bob", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Allows(context.Background(), tt.subj, res, OpenDoor); got != tt.want {
				t.Errorf("Allows(%q) = %v, want %v", tt.subj, got, tt.want)
			}
		})
	}
}

// TestAllowsResolverError — ошибка резолвера трактуется как Deny: Allows = false.
func TestAllowsResolverError(t *testing.T) {
	c, err := New[string, testResource](
		Type[string, testResource](KindDoor),
		MapScopes[string, testResource](func(r testResource) []Scope { return r.scopes }),
		Resolve[string, testResource](errAtResolver{linkErrKind: KindDoor, err: mockErr}),
		WithPolicy[string, testResource](
			Matrices{KindDoor: {roleA: right1}},
			NewScopeChain(Level(KindDoor)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := testResource{scopes: []Scope{{Kind: KindDoor, ID: 1}}}
	if got := c.Allows(context.Background(), "alice", res, right1); got {
		t.Errorf("Allows() = true, want false при ошибке резолвера")
	}
}

// TestAllowsErrZeroPerm — нулевое право даёт ErrZeroPerm: Allows = false без паники.
func TestAllowsErrZeroPerm(t *testing.T) {
	c, err := newDoorChecker()
	if err != nil {
		t.Fatalf("newDoorChecker() error = %v", err)
	}
	res := testResource{owner: "alice", scopes: []Scope{{Kind: KindDoor, ID: 7}}}
	if got := c.Allows(context.Background(), "alice", res, 0); got {
		t.Errorf("Allows(perm=0) = true, want false (ErrZeroPerm → Deny)")
	}
}

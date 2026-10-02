package rolego

import (
	"context"
	"errors"
	"testing"
)

// Тестовые виды для сценария грантов «проект — документ»: роль/грант на звене
// проекта и документа, гранты OR-ятся по звеньям вне правила оси.
const (
	kindProject Kind = 300 + iota
	kindDocument
)

// docResource — ресурс сценария грантов: собственный идентификатор (отличный от
// числовой метки Scope.ID) и звенья ресурса.
type docResource struct {
	id     uint64
	scopes []Scope
}

// docIDResolver — резолвер ролей по идентификатору документа из самого ресурса:
// роль выдаётся за поле res.id, а не за числовую метку звена Scope.ID.
type docIDResolver map[uint64]Role

func (r docIDResolver) RolesAt(_ context.Context, _ string, res docResource, link Link) (Resolved, error) {
	if link.Kind != kindDocument {
		return Resolved{}, nil
	}
	return Resolved{Roles: RolesOf(r[res.id])}, nil
}

// grantResolver — резолвер-грантодатель: ролей не выдаёт, персональное право —
// фиксированный грант.
type grantResolver struct{ grants Perm }

func (r grantResolver) RolesAt(_ context.Context, _ string, _ docResource, link Link) (Resolved, error) {
	if link.Kind != kindDocument {
		return Resolved{}, nil
	}
	return Resolved{Grants: r.grants}, nil
}

// roleGrantResolver — резолвер роли и персонального гранта одновременно.
type roleGrantResolver struct {
	role   Role
	grants Perm
}

func (r roleGrantResolver) RolesAt(_ context.Context, _ string, _ docResource, link Link) (Resolved, error) {
	if link.Kind != kindDocument {
		return Resolved{}, nil
	}
	return Resolved{Roles: RolesOf(r.role), Grants: r.grants}, nil
}

// projectGrantResolver — резолвер грантов по типу звена: на документе ролей и
// грантов нет, на проекте — персональное право right2.
type projectGrantResolver struct{}

func (projectGrantResolver) RolesAt(_ context.Context, _ string, _ docResource, link Link) (Resolved, error) {
	if link.Kind == kindProject {
		return Resolved{Grants: right2}, nil
	}
	return Resolved{}, nil
}

// docNoneResolver — резолвер без ролей и грантов (для теста причин отказа).
type docNoneResolver struct{}

func (docNoneResolver) RolesAt(context.Context, string, docResource, Link) (Resolved, error) {
	return Resolved{}, nil
}

// docErrResolver — резолвер со сбоем на звене документа.
type docErrResolver struct{ err error }

func (r docErrResolver) RolesAt(_ context.Context, _ string, _ docResource, link Link) (Resolved, error) {
	if link.Kind == kindDocument {
		return Resolved{}, r.err
	}
	return Resolved{}, nil
}

// Компиляционные проверки: резолверы сценария грантов реализуют Resolver.
var (
	_ Resolver[string, docResource] = docIDResolver{}
	_ Resolver[string, docResource] = grantResolver{}
	_ Resolver[string, docResource] = roleGrantResolver{}
	_ Resolver[string, docResource] = projectGrantResolver{}
	_ Resolver[string, docResource] = docNoneResolver{}
)

func TestRolesAtReceivesResource(t *testing.T) {
	// Резолвер читает идентификатор из самого ресурса (res.id), а не из Scope.ID:
	// метка звена — 999, поле ресурса — 1. Если бы резолвер смотрел в звено,
	// роли бы не было — ресурс действительно доходит до резолвера.
	c, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](docIDResolver{1: roleA}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for _, tt := range []struct {
		name string
		id   uint64
		want Decision
	}{
		{"роль по id документа из ресурса", 1, Allow},
		{"другой документ — роли нет", 2, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := docResource{id: tt.id, scopes: []Scope{{Kind: kindDocument, ID: 999}}}
			got, err := c.Check(context.Background(), "alice", res, right1)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%d) = %v, want %v (резолвер должен смотреть в res.id)", tt.id, got, tt.want)
			}
		})
	}
}

func TestGrantWithoutRole(t *testing.T) {
	// Грант — параллельный канал прав: ролей резолвер не выдаёт, но персональный
	// грант right2 даёт Allow по этому праву; невыписанное право — Deny.
	c, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](grantResolver{grants: right2}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := docResource{scopes: []Scope{{Kind: kindDocument, ID: 1}}}

	for _, tt := range []struct {
		name string
		perm Perm
		want Decision
	}{
		{"право из гранта — Allow", right2, Allow},
		{"право только матрицы — Deny", right1, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.Check(context.Background(), "alice", res, tt.perm)
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Check(%#x) = %v, want %v (ролей нет, работает только грант)", tt.perm, got, tt.want)
			}
		})
	}
}

func TestGrantPlusRole(t *testing.T) {
	// Роль даёт right1, грант — right2: эффективная маска right1|right2. Грант
	// добавляется поверх прав роли по OR; причина отказа для непокрытого права —
	// DenyReasonNoPerm (роли и гранты есть, права нет).
	c, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](roleGrantResolver{role: roleA, grants: right2}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := docResource{scopes: []Scope{{Kind: kindDocument, ID: 1}}}

	for _, tt := range []struct {
		name string
		perm Perm
		want Decision
	}{
		{"грант поверх роли — Allow", right2, Allow},
		{"роль сохраняет своё право — Allow", right1, Allow},
		{"непокрытое право — Deny", right3, Deny},
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

	t.Run("причина отказа при непокрытом праве — NoPerm", func(t *testing.T) {
		r, err := c.CheckResult(context.Background(), "alice", res, right3)
		if err != nil {
			t.Fatalf("CheckResult() error = %v", err)
		}
		if r.Decision != Deny {
			t.Errorf("CheckResult() = %v, want Deny", r.Decision)
		}
		if r.Reason != DenyReasonNoPerm {
			t.Errorf("CheckResult().Reason = %v, want DenyReasonNoPerm (роли есть, права нет)", r.Reason)
		}
	})
}

func TestCheckResultDenyReasons(t *testing.T) {
	// Причины отказа различимы: пустые роли/гранты → NoRoles; роли есть, но право
	// не выписано → NoPerm; сбой резолвера → ошибка, Reason незначим.
	noRolesC, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](docNoneResolver{}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New(noRoles) error = %v", err)
	}

	noPermC, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](docIDResolver{1: roleA}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New(noPerm) error = %v", err)
	}

	errC, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](docErrResolver{err: errMock}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument)),
		),
	)
	if err != nil {
		t.Fatalf("New(err) error = %v", err)
	}

	for _, tt := range []struct {
		name        string
		c           *Checker[string, docResource]
		perm        Perm
		wantReason  DenyReason
		checkReason bool // false — Reason незначим (ошибка резолвера)
	}{
		{"ролей и грантов нет — NoRoles", noRolesC, right1, DenyReasonNoRoles, true},
		{"роль есть, право не выписано — NoPerm", noPermC, right2, DenyReasonNoPerm, true},
		{"ошибка резолвера — Err, Reason незначим", errC, right1, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := docResource{id: 1, scopes: []Scope{{Kind: kindDocument, ID: 1}}}
			r, err := tt.c.CheckResult(context.Background(), "alice", res, tt.perm)
			if r.Decision != Deny {
				t.Errorf("CheckResult().Decision = %v, want Deny", r.Decision)
			}
			if tt.checkReason && r.Reason != tt.wantReason {
				t.Errorf("CheckResult().Reason = %v, want %v", r.Reason, tt.wantReason)
			}
			if tt.c == errC {
				if !errors.Is(err, errMock) {
					t.Errorf("CheckResult() error = %v, want errors.Is(errMock)", err)
				}
			} else if err != nil {
				t.Errorf("CheckResult() error = %v, want nil", err)
			}
		})
	}
}

func TestGrantWithNearestRule(t *testing.T) {
	// Гранты OR-ятся по всем звеньям вне правила оси: на документе ролей и
	// грантов нет, на проекте — грант right2. Nearest по ролям поднимается к
	// корню, но ролей нет нигде — итоговая маска пуста; грант проекта тем не
	// менее даёт Allow по right2.
	c, err := New[string, docResource](
		Type[string, docResource](kindDocument),
		MapScopes[string, docResource](func(d docResource) []Scope { return d.scopes }),
		Resolve[string, docResource](projectGrantResolver{}),
		WithPolicy[string, docResource](
			Matrices{kindDocument: {roleA: right1}},
			NewScopeChain(Level(kindDocument), Level(kindProject), Combine(Nearest)),
		),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	res := docResource{scopes: []Scope{{Kind: kindProject, ID: 1}, {Kind: kindDocument, ID: 2}}}

	for _, tt := range []struct {
		name string
		perm Perm
		want Decision
	}{
		{"грант проекта — Allow несмотря на пустые роли", right2, Allow},
		{"невыписанное право — Deny", right1, Deny},
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

package rolego

import (
	"context"
	"errors"
	"testing"
)

// mock — политика-замыкание с заранее заданным (Decision, error) и счётчиком вызовов.
type mock struct {
	dec   Decision
	err   error
	calls int
}

func (m *mock) Allow(context.Context, string, Perm, string) (Decision, error) {
	m.calls++
	return m.dec, m.err
}

// constPolicy возвращает политику, всегда выдающую заданное решение.
func constPolicy(dec Decision) *mock { return &mock{dec: dec} }

// errMock — общая тестовая ошибка для сверки через errors.Is.
var errMock = errors.New("rolego: mock error")

// errPolicy возвращает политику, всегда выдающую ошибку.
func errPolicy() *mock { return &mock{dec: Deny, err: errMock} }

func TestAllOf(t *testing.T) {
	for _, tt := range []struct {
		name string
		ps   []*mock
		want Decision
	}{
		{"все разрешили", []*mock{constPolicy(Allow), constPolicy(Allow)}, Allow},
		{"один запретил — deny", []*mock{constPolicy(Allow), constPolicy(Deny), constPolicy(Allow)}, Deny},
		{"первый же запретил", []*mock{constPolicy(Deny), constPolicy(Allow)}, Deny},
		{"пустой список — вакуумно истинно", nil, Allow},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policies := make([]Policy[string, string], len(tt.ps))
			for i := range tt.ps {
				policies[i] = tt.ps[i]
			}
			got, err := AllOf(policies...).Allow(context.Background(), "subj", 1, "res")
			if err != nil {
				t.Fatalf("AllOf.Allow() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("AllOf.Allow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAllOfShortCircuit(t *testing.T) {
	// Deny прерывает обход: последующие политики не вызываются.
	first := constPolicy(Deny)
	second := constPolicy(Allow)
	got, err := AllOf[string, string](first, second).Allow(context.Background(), "s", 1, "r")
	if err != nil {
		t.Fatalf("AllOf.Allow() error = %v", err)
	}
	if got != Deny {
		t.Errorf("AllOf.Allow() = %v, want Deny", got)
	}
	if second.calls != 0 {
		t.Errorf("second вызвана %d раз, want 0 (короткое замыкание на Deny)", second.calls)
	}
}

func TestAllOfErrorPropagates(t *testing.T) {
	wantErr := errMock
	ps := []Policy[string, string]{errPolicy(), constPolicy(Allow)}
	got, err := AllOf(ps...).Allow(context.Background(), "s", 1, "r")
	if got != Deny || !errors.Is(err, wantErr) {
		t.Errorf("AllOf.Allow() = (%v, %v), want (Deny, err)", got, err)
	}
}

func TestAny(t *testing.T) {
	for _, tt := range []struct {
		name string
		ps   []*mock
		want Decision
	}{
		{"хотя бы один разрешил", []*mock{constPolicy(Deny), constPolicy(Allow)}, Allow},
		{"первый же разрешил", []*mock{constPolicy(Allow), constPolicy(Deny)}, Allow},
		{"никто не разрешил", []*mock{constPolicy(Deny), constPolicy(Deny)}, Deny},
		{"пустой список — вакуумно ложно", nil, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			policies := make([]Policy[string, string], len(tt.ps))
			for i := range tt.ps {
				policies[i] = tt.ps[i]
			}
			got, err := Any(policies...).Allow(context.Background(), "subj", 1, "res")
			if err != nil {
				t.Fatalf("Any.Allow() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Any.Allow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAnyShortCircuit(t *testing.T) {
	// Allow прерывает обход: последующие политики не вызываются.
	first := constPolicy(Allow)
	second := constPolicy(Deny)
	got, err := Any[string, string](first, second).Allow(context.Background(), "s", 1, "r")
	if err != nil {
		t.Fatalf("Any.Allow() error = %v", err)
	}
	if got != Allow {
		t.Errorf("Any.Allow() = %v, want Allow", got)
	}
	if second.calls != 0 {
		t.Errorf("second вызвана %d раз, want 0 (короткое замыкание на Allow)", second.calls)
	}
}

func TestAnyErrorPropagates(t *testing.T) {
	wantErr := errMock
	ps := []Policy[string, string]{errPolicy(), constPolicy(Allow)}
	got, err := Any(ps...).Allow(context.Background(), "s", 1, "r")
	if got != Deny || !errors.Is(err, wantErr) {
		t.Errorf("Any.Allow() = (%v, %v), want (Deny, err)", got, err)
	}
}

func TestExcept(t *testing.T) {
	for _, tt := range []struct {
		name   string
		base   Decision
		denied []Decision
		want   Decision
	}{
		{"base allow, denied deny — allow", Allow, []Decision{Deny}, Allow},
		{"base allow, denied allow — deny (deny-wins)", Allow, []Decision{Allow}, Deny},
		{"base deny, denied deny — deny", Deny, []Decision{Deny}, Deny},
		{"base deny, denied allow — deny (base уже deny)", Deny, []Decision{Allow}, Deny},
		{"нет denied — повторяет base", Allow, nil, Allow},
		{"нет denied, base deny — deny", Deny, nil, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			denied := make([]Policy[string, string], len(tt.denied))
			for i := range tt.denied {
				denied[i] = constPolicy(tt.denied[i])
			}
			got, err := Except[string, string](constPolicy(tt.base), denied...).Allow(context.Background(), "s", 1, "r")
			if err != nil {
				t.Fatalf("Except.Allow() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Except.Allow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExceptBaseDenySkipsDenied(t *testing.T) {
	// base Deny: denied-политики не вызываются.
	base := constPolicy(Deny)
	denied := constPolicy(Allow)
	got, err := Except[string, string](base, denied).Allow(context.Background(), "s", 1, "r")
	if err != nil {
		t.Fatalf("Except.Allow() error = %v", err)
	}
	if got != Deny {
		t.Errorf("Except.Allow() = %v, want Deny", got)
	}
	if denied.calls != 0 {
		t.Errorf("denied вызвана %d раз, want 0 (base уже Deny)", denied.calls)
	}
}

func TestExceptDeniedAllowStops(t *testing.T) {
	// Первая denied с Allow останавливает проверку остальных.
	base := constPolicy(Allow)
	first := constPolicy(Allow)
	second := constPolicy(Deny)
	got, err := Except[string, string](base, first, second).Allow(context.Background(), "s", 1, "r")
	if err != nil {
		t.Fatalf("Except.Allow() error = %v", err)
	}
	if got != Deny {
		t.Errorf("Except.Allow() = %v, want Deny (deny-wins)", got)
	}
	if second.calls != 0 {
		t.Errorf("second вызвана %d раз, want 0 (остановка на первом Allow в denied)", second.calls)
	}
}

func TestExceptErrorPropagates(t *testing.T) {
	wantErr := errMock

	t.Run("ошибка в base", func(t *testing.T) {
		got, err := Except[string, string](errPolicy(), constPolicy(Deny)).Allow(context.Background(), "s", 1, "r")
		if got != Deny || !errors.Is(err, wantErr) {
			t.Errorf("Except.Allow() = (%v, %v), want (Deny, err)", got, err)
		}
	})

	t.Run("ошибка в denied", func(t *testing.T) {
		got, err := Except[string, string](constPolicy(Allow), errPolicy()).Allow(context.Background(), "s", 1, "r")
		if got != Deny || !errors.Is(err, wantErr) {
			t.Errorf("Except.Allow() = (%v, %v), want (Deny, err)", got, err)
		}
	})
}

// recording — предикат, фиксирующий переданные аргументы вызова.
type recording struct {
	gotSubj string
	gotPerm Perm
	gotRes  string
}

func (r *recording) Ok(_ context.Context, subj string, perm Perm, res string) bool {
	r.gotSubj, r.gotPerm, r.gotRes = subj, perm, res
	return true
}

func TestSingleCallsWithArgs(t *testing.T) {
	rec := &recording{}
	got, err := Single[string, string](rec.Ok).Allow(context.Background(), "alice", right2, "door")
	if err != nil {
		t.Fatalf("Single.Allow() error = %v", err)
	}
	if got != Allow {
		t.Errorf("Single.Allow() = %v, want Allow", got)
	}
	if rec.gotSubj != "alice" || rec.gotPerm != right2 || rec.gotRes != "door" {
		t.Errorf("аргументы переданы неверно: (%q, %#x, %q), want (alice, %#x, door)", rec.gotSubj, rec.gotPerm, rec.gotRes, right2)
	}
}

func TestPredicateBool(t *testing.T) {
	for _, tt := range []struct {
		name string
		val  bool
		want Decision
	}{
		{"true → Allow", true, Allow},
		{"false → Deny", false, Deny},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := Predicate[string, string](func(context.Context, string, Perm, string) bool { return tt.val })
			got, err := p.Allow(context.Background(), "s", 1, "r")
			if err != nil {
				t.Fatalf("Predicate.Allow() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Predicate.Allow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPredicateCallsWithArgs(t *testing.T) {
	var subj, res string
	var perm Perm
	called := false
	p := Predicate[string, string](func(_ context.Context, s string, pr Perm, r string) bool {
		called, subj, perm, res = true, s, pr, r
		return true
	})
	if _, err := p.Allow(context.Background(), "bob", right3, "vault"); err != nil {
		t.Fatalf("Predicate.Allow() error = %v", err)
	}
	if !called || subj != "bob" || perm != right3 || res != "vault" {
		t.Errorf("аргументы переданы неверно: called=%v (%q, %#x, %q)", called, subj, perm, res)
	}
}

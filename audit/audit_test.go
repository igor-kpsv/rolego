package audit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/audit"
)

// Сценарий «ключ — дверь»: роль — держатель ключа (roleKeyholder), право —
// открыть дверь (openDoor), ось — одно звено KindDoor. Ресурс door — обычная
// структура: обёртке audit comparable не требуется.
const (
	kindDoor rolego.Kind = 210

	roleKeyholder rolego.Role = 1 << 10
	openDoor      rolego.Perm = 1 << 5
)

// door — ресурс сценария: звенья достаёт экстрактор, поле id различает двери.
type door struct {
	id uint64
}

func doorScopes(d door) []rolego.Scope {
	return []rolego.Scope{{Kind: kindDoor, ID: d.id}}
}

// mapResolver — резолвер ролей по карте «субъект → маска ролей»; неизвестный
// субъект получает пустую маску.
type mapResolver map[string]rolego.Role

func (r mapResolver) RolesAt(_ context.Context, subj string, link rolego.Link) (rolego.Roles, error) {
	if link.Kind == kindDoor {
		return rolego.RolesOf(r[subj]), nil
	}
	return rolego.RolesOf(0), nil
}

// newChecker собирает подлинный checker сценария: alice — держатель ключа.
func newChecker() *rolego.Checker[string, door] {
	c, err := rolego.New[string, door](
		rolego.Type[string, door](kindDoor),
		rolego.MapScopes[string, door](doorScopes),
		rolego.Resolve[string, door](mapResolver{"alice": roleKeyholder}),
		rolego.WithPolicy[string, door](
			rolego.Matrices{kindDoor: {roleKeyholder: openDoor}},
			rolego.NewScopeChain(rolego.Level(kindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}
	return c
}

// spyLogger — логгер-накопитель: сохраняет все события аудита.
type spyLogger struct {
	entries []audit.Entry
}

func (s *spyLogger) Log(_ context.Context, e audit.Entry) {
	s.entries = append(s.entries, e)
}

// countingLogger — логгер-счётчик: считает события, вход не анализирует.
type countingLogger struct{ n int }

func (c *countingLogger) Log(context.Context, audit.Entry) { c.n++ }

func TestCheckLogsAllow(t *testing.T) {
	spy := &spyLogger{}
	w := audit.Log(newChecker(), spy)
	res := door{id: 7}

	got, err := w.Check(context.Background(), "alice", res, openDoor)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !got.Allow() {
		t.Fatalf("Check() = %v, want Allow", got)
	}

	if len(spy.entries) != 1 {
		t.Fatalf("залогировано %d событий, want 1", len(spy.entries))
	}
	e := spy.entries[0]
	if e.Subj != "alice" {
		t.Errorf("Entry.Subj = %v, want alice", e.Subj)
	}
	if e.Res != res {
		t.Errorf("Entry.Res = %v, want %v", e.Res, res)
	}
	if e.Perm != openDoor {
		t.Errorf("Entry.Perm = %v, want %v", e.Perm, openDoor)
	}
	if e.Decision != rolego.Allow {
		t.Errorf("Entry.Decision = %v, want Allow", e.Decision)
	}
	if e.Err != nil {
		t.Errorf("Entry.Err = %v, want nil", e.Err)
	}
}

func TestCheckLogsDeny(t *testing.T) {
	spy := &spyLogger{}
	w := audit.Log(newChecker(), spy)
	res := door{id: 7}

	got, err := w.Check(context.Background(), "bob", res, openDoor)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if got.Allow() {
		t.Fatalf("Check() = %v, want Deny", got)
	}

	if len(spy.entries) != 1 {
		t.Fatalf("залогировано %d событий, want 1", len(spy.entries))
	}
	if e := spy.entries[0]; e.Decision != rolego.Deny || e.Err != nil {
		t.Errorf("Entry = %+v, want Decision=Deny и Err=nil", e)
	}
}

func TestCheckLogsError(t *testing.T) {
	spy := &spyLogger{}
	w := audit.Log(newChecker(), spy)
	res := door{id: 7}

	got, err := w.Check(context.Background(), "alice", res, 0)
	if got.Allow() {
		t.Fatalf("Check() = %v, want Deny при ErrZeroPerm", got)
	}
	if !errors.Is(err, rolego.ErrZeroPerm) {
		t.Fatalf("Check() error = %v, want ErrZeroPerm (сверка errors.Is)", err)
	}

	if len(spy.entries) != 1 {
		t.Fatalf("залогировано %d событий, want 1", len(spy.entries))
	}
	e := spy.entries[0]
	if e.Decision != rolego.Deny {
		t.Errorf("Entry.Decision = %v, want Deny", e.Decision)
	}
	if e.Err == nil {
		t.Error("Entry.Err = nil, want не-nil")
	}
	if !errors.Is(e.Err, rolego.ErrZeroPerm) {
		t.Errorf("Entry.Err = %v, want ErrZeroPerm (сверка errors.Is)", e.Err)
	}
}

func TestWhoCanAndValidateDelegate(t *testing.T) {
	counter := &countingLogger{}
	w := audit.Log(newChecker(), counter)

	if err := w.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	got, err := w.WhoCan(context.Background(), []string{"alice", "bob"}, door{id: 1}, openDoor)
	if err != nil {
		t.Fatalf("WhoCan() error = %v", err)
	}
	if len(got) != 1 || got[0] != "alice" {
		t.Errorf("WhoCan() = %v, want [alice]", got)
	}

	if counter.n != 0 {
		t.Errorf("залогировано %d событий, want 0 (WhoCan и Validate не порождают аудит)", counter.n)
	}
}

func TestLogNilLoggerPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Log() с nil-логгером не запаниковал, want panic")
		}
	}()
	audit.Log(newChecker(), nil)
}

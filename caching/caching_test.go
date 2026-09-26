package caching_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/caching"
)

// Сценарий «ключ — дверь»: роль — держатель ключа (roleKeyholder), право —
// открыть дверь (openDoor), ось — одно звено KindDoor. Ресурс door comparable:
// звенья (одна дверь) производятся экстрактором из id детерминированно.
const (
	kindDoor rolego.Kind = 210

	roleKeyholder rolego.Role = 1 << 10
	openDoor      rolego.Perm = 1 << 5
)

// door — comparable-ресурс кэша: звенья достаёт экстрактор, поле id — часть ключа.
type door struct {
	id uint64
}

func doorScopes(d door) []rolego.Scope {
	return []rolego.Scope{{Kind: kindDoor, ID: d.id}}
}

var errResolver = errors.New("caching: resolver failure")

// countingResolver — резолвер-счётчик: число вызовов равно числу уникальных
// промахов кэша, дошедших до подлинного checker. При err != nil каждый вызов
// возвращает ошибку.
type countingResolver struct {
	calls int
	keys  map[string]struct{}
	err   error
}

func (r *countingResolver) RolesAt(_ context.Context, subj string, link rolego.Link) (rolego.Roles, error) {
	r.calls++
	if r.err != nil {
		return rolego.RolesOf(0), r.err
	}
	if link.Kind == kindDoor {
		if _, ok := r.keys[subj]; ok {
			return rolego.RolesOf(roleKeyholder), nil
		}
	}
	return rolego.RolesOf(0), nil
}

// newChecker собирает подлинный checker сценария и резолвер-счётчик.
func newChecker() (*rolego.Checker[string, door], *countingResolver) {
	rr := &countingResolver{keys: map[string]struct{}{"alice": {}}}
	c, err := rolego.New[string, door](
		rolego.Type[string, door](kindDoor),
		rolego.MapScopes[string, door](doorScopes),
		rolego.Resolve[string, door](rr),
		rolego.WithPolicy[string, door](
			rolego.Matrices{kindDoor: {roleKeyholder: openDoor}},
			rolego.NewScopeChain(rolego.Level(kindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}
	return c, rr
}

func TestCheckCacheHit(t *testing.T) {
	inner, rr := newChecker()
	w := caching.Wrap(inner)
	res := door{id: 7}

	for i := 0; i < 3; i++ {
		got, err := w.Check(context.Background(), "alice", res, openDoor)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if !got.Allow() {
			t.Fatalf("Check() = %v, want Allow", got)
		}
	}
	if rr.calls != 1 {
		t.Errorf("резолвер вызван %d раз, want 1 (повторные Check — кэш-хит)", rr.calls)
	}

	// Deny без ошибки — тоже решение: кэшируется, резолвер не тревожится повторно.
	if got, err := w.Check(context.Background(), "bob", res, openDoor); err != nil || got.Allow() {
		t.Fatalf("Check(bob) = %v, %v, want Deny без ошибки", got, err)
	}
	if got, err := w.Check(context.Background(), "bob", res, openDoor); err != nil || got.Allow() {
		t.Fatalf("Check(bob, повтор) = %v, %v, want Deny без ошибки", got, err)
	}
	if rr.calls != 2 {
		t.Errorf("резолвер вызван %d раз, want 2 (alice и bob по одному промаху)", rr.calls)
	}
}

func TestCheckTTLExpiry(t *testing.T) {
	inner, rr := newChecker()
	w := caching.Wrap(inner, caching.WithTTL(20*time.Millisecond))
	res := door{id: 1}

	if _, err := w.Check(context.Background(), "alice", res, openDoor); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if _, err := w.Check(context.Background(), "alice", res, openDoor); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if rr.calls != 1 {
		t.Fatalf("резолвер вызван %d раз, want 1 до истечения TTL", rr.calls)
	}

	time.Sleep(50 * time.Millisecond)
	if _, err := w.Check(context.Background(), "alice", res, openDoor); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if rr.calls != 2 {
		t.Errorf("резолвер вызван %d раз, want 2 (после истечения TTL — снова промах)", rr.calls)
	}
}

func TestClear(t *testing.T) {
	inner, rr := newChecker()
	w := caching.Wrap(inner)
	res := door{id: 1}

	for i := 0; i < 2; i++ {
		if _, err := w.Check(context.Background(), "alice", res, openDoor); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	if rr.calls != 1 {
		t.Fatalf("резолвер вызван %d раз, want 1 до Clear", rr.calls)
	}

	w.Clear()
	if w.Len() != 0 {
		t.Errorf("Len() после Clear = %d, want 0", w.Len())
	}
	if _, err := w.Check(context.Background(), "alice", res, openDoor); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if rr.calls != 2 {
		t.Errorf("резолвер вызван %d раз, want 2 (Clear сбросил кэш)", rr.calls)
	}
}

func TestLen(t *testing.T) {
	inner, _ := newChecker()
	w := caching.Wrap(inner)

	for _, subj := range []string{"alice", "bob", "carol"} {
		if _, err := w.Check(context.Background(), subj, door{id: 1}, openDoor); err != nil {
			t.Fatalf("Check(%s) error = %v", subj, err)
		}
	}
	if got := w.Len(); got != 3 {
		t.Errorf("Len() = %d, want 3 (записи по разным субъектам)", got)
	}
}

func TestMaxEntriesEviction(t *testing.T) {
	inner, rr := newChecker()
	w := caching.Wrap(inner, caching.WithMaxEntries(2))

	for id := uint64(1); id <= 3; id++ {
		if _, err := w.Check(context.Background(), "alice", door{id: id}, openDoor); err != nil {
			t.Fatalf("Check(door %d) error = %v", id, err)
		}
	}
	if got := w.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 (лимит записей)", got)
	}

	// Самая старая запись (door 1) вытеснена — повторный Check снова трогает резолвер.
	if _, err := w.Check(context.Background(), "alice", door{id: 1}, openDoor); err != nil {
		t.Fatalf("Check(door 1) error = %v", err)
	}
	if rr.calls != 4 {
		t.Errorf("резолвер вызван %d раз, want 4 (первый промах после эвикции)", rr.calls)
	}
}

func TestErrorNotCached(t *testing.T) {
	inner, rr := newChecker()
	rr.err = errResolver
	w := caching.Wrap(inner)
	res := door{id: 1}

	for i := 0; i < 2; i++ {
		got, err := w.Check(context.Background(), "alice", res, openDoor)
		if got.Allow() {
			t.Fatalf("Check() = %v, want Deny при ошибке резолвера", got)
		}
		if !errors.Is(err, errResolver) {
			t.Fatalf("Check() error = %v, want %v (сверка errors.Is)", err, errResolver)
		}
	}
	if rr.calls != 2 {
		t.Errorf("резолвер вызван %d раз, want 2 (ошибка не кэшируется)", rr.calls)
	}
	if got := w.Len(); got != 0 {
		t.Errorf("Len() = %d, want 0 (ошибочные проверки в кэш не пишутся)", got)
	}
}

func TestDelegate(t *testing.T) {
	inner, _ := newChecker()
	w := caching.Wrap(inner)

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
}

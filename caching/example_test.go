package caching_test

import (
	"context"
	"fmt"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/caching"
)

// ExampleWrap — обёртка «ключ — дверь»: повторный Check того же ключа не
// опрашивает резолвер заново — число вызовов резолвера остаётся 1.
func ExampleWrap() {
	rr := &countingResolver{keys: map[string]struct{}{"alice": {}}}
	checker, err := rolego.New[string, door](
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
	wrapped := caching.Wrap(checker, caching.WithMaxEntries(100))

	d := door{id: 7}
	first, _ := wrapped.Check(context.Background(), "alice", d, openDoor)
	second, _ := wrapped.Check(context.Background(), "alice", d, openDoor)
	fmt.Println(first.Allow(), second.Allow(), rr.calls)

	// Output:
	// true true 1
}

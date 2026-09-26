package audit_test

import (
	"context"
	"fmt"

	"github.com/igor-kpsv/rolego"
	"github.com/igor-kpsv/rolego/audit"
)

// printLogger — логгер примера: печатает субъект, вердикт и ошибку события.
type printLogger struct{}

func (printLogger) Log(_ context.Context, e audit.Entry) {
	verdict := "Deny"
	if e.Decision.Allow() {
		verdict = "Allow"
	}
	fmt.Printf("%v=%s err=%v\n", e.Subj, verdict, e.Err)
}

// ExampleLog показывает обёртку audit.Log: каждое решение Check доходит до
// логгера, вызывающий получает решение и ошибку без изменений.
func ExampleLog() {
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
	w := audit.Log(c, printLogger{})

	for _, subj := range []string{"alice", "bob"} {
		_, _ = w.Check(context.Background(), subj, door{id: 1}, openDoor)
	}
	// Output:
	// alice=Allow err=<nil>
	// bob=Deny err=<nil>
}

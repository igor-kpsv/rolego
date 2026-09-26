package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/igor-kpsv/rolego"
)

// ExampleRequire — middleware «ключ — дверь»: parse читает субъекта из заголовка
// X-User, роль держателя ключа открывает дверь, остальные получают 403.
func ExampleRequire() {
	checker, err := rolego.New[string, doorResource](
		rolego.Type[string, doorResource](kindDoor),
		rolego.MapScopes[string, doorResource](func(d doorResource) []rolego.Scope { return d.scopes }),
		rolego.Resolve[string, doorResource](holderResolver{"alice": roleKeyholder}),
		rolego.WithPolicy[string, doorResource](
			rolego.Matrices{kindDoor: {roleKeyholder: openDoor}},
			rolego.NewScopeChain(rolego.Level(kindDoor)),
		),
	)
	if err != nil {
		panic(err)
	}

	parse := func(r *http.Request) (string, doorResource, error) {
		user := r.Header.Get("X-User")
		if user == "" {
			return "", doorResource{}, errors.New("empty subject")
		}
		return user, doorResource{scopes: []rolego.Scope{{Kind: kindDoor, ID: 7}}}, nil
	}

	handler := Require(checker, openDoor, parse)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("open"))
	}))

	for _, user := range []string{"alice", "bob"} {
		req := httptest.NewRequest(http.MethodGet, "/door/7", nil)
		req.Header.Set("X-User", user)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		fmt.Printf("%s: %d\n", user, rec.Code)
	}

	// Output:
	// alice: 200
	// bob: 403
}

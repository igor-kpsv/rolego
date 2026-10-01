package rolego

import (
	"errors"
	"testing"
)

func TestRegistryRoundTrip(t *testing.T) {
	reg := new(Registry[Perm])
	reg.Register("documents.view", right1)
	reg.Register("documents.edit", right1|right2)

	if v, err := reg.Parse("documents.view"); err != nil || v != right1 {
		t.Errorf("Parse(view) = %#x, %v; want %#x, nil", v, err, right1)
	}
	if v, err := reg.Parse("documents.edit"); err != nil || v != right1|right2 {
		t.Errorf("Parse(edit) = %#x, %v; want %#x, nil", v, err, right1|right2)
	}
	if v := reg.MustParse("documents.edit"); v != right1|right2 {
		t.Errorf("MustParse(edit) = %#x, want %#x", v, right1|right2)
	}

	if name, ok := reg.Name(right1); !ok || name != "documents.view" {
		t.Errorf("Name(right1) = %q, %v; want documents.view, true", name, ok)
	}
	if name, ok := reg.Name(right1 | right2); !ok || name != "documents.edit" {
		t.Errorf("Name(составное) = %q, %v; want documents.edit, true", name, ok)
	}
	if _, ok := reg.Name(right1 << 10); ok {
		t.Errorf("Name(незарегистрированный бит) ok = %v, want false", ok)
	}
}

func TestRegistryNext(t *testing.T) {
	reg := new(Registry[Role])
	a := reg.Next("viewer")
	b := reg.Next("editor")
	c := reg.Next("manager")

	if a != roleA || b != roleB || c != roleC {
		t.Errorf("Next() = %#x, %#x, %#x; want %#x, %#x, %#x (совместимо с 1<<iota)",
			a, b, c, roleA, roleB, roleC)
	}
	if name, _ := reg.Name(roleC); name != "manager" {
		t.Errorf("Name(roleC) = %q, want manager", name)
	}
}

func TestRegistryNextSkipsTaken(t *testing.T) {
	// Next пропускает бит, занятый явным Register: первый свободный.
	reg := new(Registry[Perm])
	reg.Register("explicit", right1)
	if v := reg.Next("auto"); v != right2 {
		t.Errorf("Next() после Register(right1) = %#x, want %#x", v, right2)
	}
}

func TestRegistryNextSkipsCompositeBits(t *testing.T) {
	// Составной Register(right1|right2) резервирует оба бита: Next обязан выдать
	// первый свободный за пределами маски, а не тихо занять right1 внутри неё.
	reg := new(Registry[Perm])
	reg.Register("documents.edit", right1|right2)

	if v := reg.Next("audit.log"); v != right3 {
		t.Errorf("Next() после Register(right1|right2) = %#x, want %#x", v, right3)
	}
	if name, ok := reg.Name(right3); !ok || name != "audit.log" {
		t.Errorf("Name(right3) = %q, %v; want audit.log, true", name, ok)
	}
	if _, ok := reg.Name(right1); ok {
		t.Errorf("Name(right1) ok = true, want false: бит внутри составной маски отдельной записью не стал")
	}
}

func TestRegistryNextMixedWithComposite(t *testing.T) {
	// Смешение автонумерации и составного Register в любом порядке: Next выдаёт
	// реально свободный бит, не пересекающийся ни с одной зарегистрированной
	// маской. Составная занимает биты 1 и 3 — свободен бит 2 (right3).
	reg := new(Registry[Perm])
	a := reg.Next("base")                              // 1<<0
	reg.Register("documents.edit", right2|(right3<<1)) // 1<<1 | 1<<3
	b := reg.Next("after")

	if a != right1 {
		t.Errorf("Next(base) = %#x, want %#x", a, right1)
	}
	if b != right3 {
		t.Errorf("Next(after) = %#x, want %#x (бит 2 свободен между занятыми 1 и 3)", b, right3)
	}
	if name, ok := reg.Name(right3); !ok || name != "after" {
		t.Errorf("Name(right3) = %q, %v; want after, true", name, ok)
	}
}

func TestRegistryParseUnknown(t *testing.T) {
	reg := new(Registry[Perm])
	v, err := reg.Parse("no.such")
	if v != 0 {
		t.Errorf("Parse(unknown) = %#x, want 0", v)
	}
	if !errors.Is(err, ErrUnknownRegistryName) {
		t.Errorf("Parse(unknown) error = %v, want errors.Is(ErrUnknownRegistryName)", err)
	}
}

func TestRegistryDuplicatePanics(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  func()
	}{
		{
			"повторное имя",
			func() {
				reg := new(Registry[Perm])
				reg.Register("x", right1)
				reg.Register("x", right2)
			},
		},
		{
			"повторный бит под другим именем",
			func() {
				reg := new(Registry[Perm])
				reg.Register("x", right1)
				reg.Register("y", right1)
			},
		},
		{
			"повторное имя для Next",
			func() {
				reg := new(Registry[Perm])
				reg.Next("x")
				reg.Next("x")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("ожидалась паника")
				}
			}()
			tt.run()
		})
	}
}

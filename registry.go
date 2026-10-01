package rolego

import "fmt"

// Registry — именной реестр битов: строковое имя ↔ бит в типе T (Perm или Role).
// Пригодится при внедрении поверх унаследованных строковых прав: строки из БД
// (<домен>.<действие> — для реестра просто непрозрачное имя) конвертятся в биты
// без миграции схемы. Реестр бездоменен: разбивать строку на части он не пытается.
//
// Нулевое значение рабочее. Реестр не потокобезопасен: заполняется один раз при
// старте, дальше только читается.
type Registry[T ~uint64] struct {
	byName map[string]T
	names  map[T]string
	// used — OR всех зарегистрированных масок: бит занят, если входит хотя бы в
	// одну зарегистрированную маску (в том числе составную). Next опирается на
	// него, а не на точные ключи names: составной Register(right1|right2) должен
	// резервировать для автонумерации и right1, и right2.
	used T
}

// Register привязывает имя name к уже объявленному биту v (константе роли или
// права). Повторное имя или повторный бит — паника: рассинхрон реестра с
// константами кода опаснее паники в тесте.
func (r *Registry[T]) Register(name string, v T) {
	r.init()
	if _, ok := r.byName[name]; ok {
		panic("rolego: registry name already registered: " + name)
	}
	if prev, ok := r.names[v]; ok {
		panic(fmt.Sprintf("rolego: registry bit 0x%x already used by %q", uint64(v), prev))
	}
	r.byName[name] = v
	r.names[v] = name
	r.used |= v
}

// Next регистрирует name за первым свободным битом (начиная с 1<<0, пропуская
// занятые) и возвращает его. Занятым считается бит, входящий в любую
// зарегистрированную маску — включая составную: Register(right1|right2)
// резервирует оба бита, так что Next выдаст бит за их пределами. Порядок
// детерминирован по порядку вызовов — совместим с объявлениями 1 << iota.
// Повторное имя — паника.
func (r *Registry[T]) Next(name string) T {
	r.init()
	if _, ok := r.byName[name]; ok {
		panic("rolego: registry name already registered: " + name)
	}
	var v T = 1
	for v != 0 {
		if v&r.used == 0 {
			r.byName[name] = v
			r.names[v] = name
			r.used |= v
			return v
		}
		v <<= 1
	}
	panic("rolego: registry bits exhausted")
}

// Parse возвращает бит по имени; неизвестное имя — (0, ErrUnknownRegistryName),
// сверяется через errors.Is.
func (r *Registry[T]) Parse(s string) (T, error) {
	if v, ok := r.byName[s]; ok {
		return v, nil
	}
	return 0, fmt.Errorf("%w: %q", ErrUnknownRegistryName, s)
}

// MustParse — Parse, паникующий при неизвестном имени; для конфигурационных
// строк, которые обязаны существовать.
func (r *Registry[T]) MustParse(s string) T {
	v, err := r.Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// Name возвращает имя по биту; не зарегистрированный бит — ok == false.
// Обратный ход пригодится для аудит-логов: из числа Perm в читаемое имя.
func (r *Registry[T]) Name(v T) (string, bool) {
	name, ok := r.names[v]
	return name, ok
}

func (r *Registry[T]) init() {
	if r.byName == nil {
		r.byName = make(map[string]T)
		r.names = make(map[T]string)
	}
}

package rolego

// Role — роль субъекта как битовая маска; роли объявляются как const R Role = 1 << iota.
type Role uint64

// Perm — право как битовая маска; права объявляются как const P Perm = 1 << iota.
type Perm uint64

// All — все права сразу: все биты Perm установлены.
const All = ^Perm(0)

// Roles — множество ролей субъекта как битовая маска.
type Roles struct{ bits Role }

// RolesOf возвращает множество ролей из маски role (обычно один бит).
func RolesOf(role Role) Roles { return Roles{bits: role} }

// Has проверяет, что роль role (или любой бит составной маски) есть в множестве.
func (r Roles) Has(role Role) bool { return r.bits&role != 0 }

// HasAny проверяет, что множество пересекается с маской set.
func (r Roles) HasAny(set Role) bool { return r.bits&set != 0 }

// HasAll проверяет, что маска set целиком содержится в множестве.
func (r Roles) HasAll(set Role) bool { return r.bits&set == set }

// Add возвращает копию множества с добавленной ролью role (идемпотентно).
func (r Roles) Add(role Role) Roles { return Roles{bits: r.bits | role} }

// Remove возвращает копию множества без роли role (no-op, если роли не было).
func (r Roles) Remove(role Role) Roles { return Roles{bits: r.bits &^ role} }

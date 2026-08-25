package locale

const (
	PluralZero  PluralForm = "zero"
	PluralOne   PluralForm = "one"
	PluralTwo   PluralForm = "two"
	PluralFew   PluralForm = "few"
	PluralMany  PluralForm = "many"
	PluralOther PluralForm = "other"
)

type PluralForm string

func (f PluralForm) String() string { return string(f) }

type PluralFunc func(amount int) PluralForm

type Pluralizer struct {
	Func  PluralFunc
	Forms []PluralForm
}

// IMPROVE if possible
func probePluralForms(f PluralFunc) []PluralForm {
	candidates := []int{
		0, 1, 2, 3, 4, 5, 6,
		10, 11, 12, 13, 14, 15,
		20, 21, 22,
		100, 101, 102, 111,
	}
	seen := map[PluralForm]struct{}{}
	for _, n := range candidates {
		seen[f(n)] = struct{}{}
	}

	result := make([]PluralForm, 0, len(seen))
	for k := range seen {
		result = append(result, k)
	}

	return result
}

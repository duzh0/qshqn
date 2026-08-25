package locale

var (
	English = register("en", "🇺🇸", []string{"qshqn", "kshkun"},
		Name{
			Local:   "english",
			English: "english",
		},

		func(amount int) PluralForm {
			if amount == 1 {
				return PluralOne
			}
			return PluralOther
		},
		[]Case{
			CaseNom,
		},
	)

	Ukrainian = register("uk", "🇺🇦", []string{"кшкун"},
		Name{
			Local:   "українська",
			English: "ukrainian",
		},
		func(amount int) PluralForm {
			modTen := amount % 10
			modHund := amount % 100
			if modTen == 1 && modHund != 11 {
				return PluralOne
			} else if modTen >= 2 && modTen <= 4 && !(modHund >= 11 && modHund <= 14) {
				return PluralFew
			} else if modTen == 0 || modTen >= 5 || modHund >= 11 {
				return PluralMany
			}
			return PluralOther
		},
		[]Case{
			CaseNom,
			CaseGen,
			CaseDat,
			CaseAcc,
			CaseIns,
			CaseLoc,
			CaseVoc,
		},
	)
)

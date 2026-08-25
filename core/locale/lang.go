package locale

import (
	"fmt"
)

const EXAMPLE_LOCALE_LANG_CODE = "example"

var (
	langs = map[LangCode]*Lang{}

	defaultTriggers = []string{"кшкун", "kshkun", "qshqn", "kškun", "qšqn"}
	triggers        = make(map[string]struct{}, len(defaultTriggers))

	BaseLang = English
)

type InitOptions struct {
	LocalePaths map[LangCode]string
}

type LangCode string

func (lc LangCode) Lang() *Lang    { return langs[lc] }
func (lc LangCode) String() string { return string(lc) }

type Name struct {
	Local, English string
}

type Lang struct {
	Code             LangCode
	Name             Name
	Flag             string
	PreferredTrigger string
	SupportedCases   []Case
	Pluralizer       *Pluralizer
}

func register(code LangCode, flag string, triggersList []string, name Name, pluralFunc PluralFunc, supportedCases []Case) *Lang {
	if code == "" {
		panic("tried to create a lang with empty string code")
	}

	if code == EXAMPLE_LOCALE_LANG_CODE {
		panic(fmt.Sprintf("tried to create a lang with code [%s] reserved for example locale file", EXAMPLE_LOCALE_LANG_CODE))
	}

	if len(triggersList) < 1 {
		panic(fmt.Sprintf("tried to create a lang with code [%s] without trigger words", code))
	}

	if _, ok := langs[code]; ok {
		panic(fmt.Sprintf("tried assigning lang code [%s] more than once", code))
	}

	l := &Lang{
		Code:             code,
		Name:             name,
		Flag:             flag,
		PreferredTrigger: triggersList[0],
		SupportedCases:   supportedCases,
		Pluralizer: &Pluralizer{
			Func:  pluralFunc,
			Forms: probePluralForms(pluralFunc),
		},
	}

	langs[code] = l
	for _, trigger := range triggersList {
		triggers[trigger] = struct{}{}
	}

	return l
}

func SupportedCode[T ~string](code T) (*Lang, bool) {
	l, ok := langs[LangCode(code)]
	return l, ok
}

func IsTrigger(word string) bool {
	_, ok := triggers[word]
	return ok
}

func AllLangs() map[LangCode]*Lang {
	return langs
}

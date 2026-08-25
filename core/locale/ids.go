package locale

import "slices"

const (
	RootIDGlobal = "global"
	RootIDTalk   = "talk"
	RootIDHelp   = "help"
)

var (
	GlobalIDs = func() GlobalMsgIDs {
		g := GlobalMsgIDs{
			Nouns: Nouns{
				Duzhocoins: NewNounTemplate("duzhocoins"),
			},
		}
		if err := FillMsgIDs(RootIDGlobal, &g); err != nil {
			panic("error filling global msg ids: " + err.Error())
		}
		return g
	}()
)

type SharedMsgIDs struct {
	ApproveAddChatButton,
	DeclineAddChatButton MsgID
}

type GlobalMsgIDs struct {
	Error,
	RuNames,

	InlineDuzhocoinsTitle,
	InlineDuzhocoinsDescription,
	InlineDuzhocoinsMessageResult,

	TimeHourShort,
	TimeMinuteShort,
	TimeSecondShort,

	NotCallbackButtonOwner,

	CustomSystemPromptStart,
	CustomSystemPromptEnd,
	UserNameStart,
	UserNameEnd,

	WhitelistNotSupergroup,
	WhitelistNoticeMsg,
	WhitelistRequestOwner,
	WhitelistUserJoinedMsg,
	WhitelistWelcomeMsg,
	WhitelistApprovedOwner,
	WhitelistApprovedUser,
	WhitelistDeclinedOwner,
	WhitelistDeclinedUser MsgID

	Nouns Nouns
}

type Nouns struct {
	Duzhocoins NounTemplate
}

type NounTemplate string

func (t NounTemplate) Construct(code LangCode, caseID Case, number int) MsgID {
	form := code.Lang().Pluralizer.Func(number)

	withCase := MakeMsgID(t, caseID)
	withForm := MakeMsgID(withCase, form)

	return withForm
}

// KOSTYL should be changed to allow languages to define case in locale files in the future
func (t NounTemplate) ConstructSafe(code LangCode, caseID Case, number int) MsgID {
	lang := code.Lang()
	if !slices.Contains(lang.SupportedCases, caseID) {
		caseID = CaseNom
	}
	return t.Construct(code, caseID, number)
}

// returns all msg ids taking into account Lang's supported cases and plural forms
//
// ex: template := "test"; lang := English; returns ["test.nom.one", "test.nom.other"] (one case and two possible plural forms)
func (t NounTemplate) AllIDs(code LangCode) []MsgID {
	var ids []MsgID
	lang := code.Lang()
	for _, caseID := range lang.SupportedCases {
		for _, form := range lang.Pluralizer.Forms {
			caseStr := MakeMsgID(t, caseID)
			formStr := MakeMsgID(caseStr, form)
			ids = append(ids, formStr)
		}
	}
	return ids
}

func NewNounTemplate(baseID string) NounTemplate {
	return NounTemplate(baseID)
}

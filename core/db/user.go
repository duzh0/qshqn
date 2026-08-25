package db

import "qshqn/core/locale"

const userTable = "tg_users"

type User struct {
	ID                 int64           `db:"id,pk"`
	AH                 int64           `db:"ah"` // access hash
	Banned             bool            `db:"banned"`
	LangCode           locale.LangCode `db:"lang_code"`
	Duzhocoins         int64           `db:"duzhocoins"`
	Dodep              int64           `db:"dodep"`
	DodepTimestamp     int64           `db:"dodep_timestamp"`
	MenstraDate        string          `db:"menstra_date"`
	CustomSystemPrompt string          `db:"custom_system_prompt"`
}

func (u User) isEntity()    {}
func (u *User) Save() error { return Save(u) }

func init() {
	register(userTable, func(ID int64) *User {
		return &User{
			ID:                 ID,
			LangCode:           locale.Ukrainian.Code,
			Duzhocoins:         0,
			Dodep:              0,
			DodepTimestamp:     0,
			MenstraDate:        "",
			CustomSystemPrompt: "",
		}
	})
}

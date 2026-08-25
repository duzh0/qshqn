package db

const chatTable = "tg_chats"

type Chat struct {
	ID             int64  `db:"id,pk"`
	Whitelisted    bool   `db:"whitelisted"`
	WhoreBotOn     bool   `db:"whorebot_on"`
	WhoreBotPrompt string `db:"whorebot_prompt"`
}

func (Chat) isEntity()      {}
func (c *Chat) Save() error { return Save(c) }

func init() {
	register(chatTable, func(ID int64) *Chat {
		return &Chat{
			ID:             ID,
			Whitelisted:    false,
			WhoreBotOn:     false,
			WhoreBotPrompt: "",
		}
	})
}

package config

type InitFile struct {
	Tg       TgValues       `json:"tg"`
	Db       DbValues       `json:"db"`
	Predator PredatorValues `json:"predator"`
	Services ServicesValues `json:"services"`
	Links    LinksValues    `json:"links"`
}

func (f *InitFile) Validate() error {
	return ValidateStruct(f)
}

func GetExampleInitFile() InitFile {
	return InitFile{
		Tg: TgValues{
			Creds: TgCreds{
				AppID:    12345,
				AppHash:  "example_hash",
				BotToken: "example_bot_token",
			},
			SessionPath: "example.session",
			OwnerID:     0,
			Autoreplies: map[string]string{
				"golang":     "golang|[гґ]олан[гґ]",
				"luhansk":    "луганськ|луганск|луганщ|luhansk",
				"petushok":   "петушок|петуч|петух|петушк|петуша|петуши|півень|півник|русск|россий|россия|russia|белоруссия|молдавия|лнр|днр|🇷🇺|🐓|🐔",
				"nixos":      "nixos|ніксос|никсос",
				"predator":   "predator|предатор",
				"rusnia_gif": "ы|э|ъ|ё|ѣ|🇷🇺",
			},
		},
		Db: DbValues{
			Path: "example.db",
		},
		Predator: PredatorValues{
			Msgs: PredatorMsgs{
				Path:       "example_predator_msgs.json",
				ImportMode: PREDATOR_MSGS_IMPORT_MODE_UNSPECIFIED,
				AllImportModes: PredatorMsgsImportModes{
					Unspecified: PREDATOR_MSGS_IMPORT_MODE_UNSPECIFIED,
					Skip:        PREDATOR_MSGS_IMPORT_MODE_SKIP,
					Update:      PREDATOR_MSGS_IMPORT_MODE_UPDATE,
					Replace:     PREDATOR_MSGS_IMPORT_MODE_REPLACE,
				},
			},
		},
		Services: ServicesValues{
			Gemini: GeminiService{
				APIKeys:      []string{"example_key_1", "example_key_2"},
				DefaultModel: "gemini-3.5-flash-lite",
			},
			Weather: WeatherService{
				Token: "example_weather_token",
			},
		},
		Links: LinksValues{
			DuzhocoinsImage:          "example_duzhocoins_image",
			RandomChatBubbleImage:    "example_random_chat_bubble_image",
			GeneratedChatBubbleImage: "example_generated_chat_bubble_image",
		},
	}
}

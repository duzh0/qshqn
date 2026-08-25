package config

const (
	INIT_FILE_PATH         = "init.json"
	EXAMPLE_INIT_FILE_PATH = "init_example.json"

	PREDATOR_MSGS_IMPORT_MODE_UNSPECIFIED = ""
	PREDATOR_MSGS_IMPORT_MODE_SKIP        = "skip"
	PREDATOR_MSGS_IMPORT_MODE_UPDATE      = "update"
	PREDATOR_MSGS_IMPORT_MODE_REPLACE     = "replace"
)

var (
	RU_FLAGS            = []string{"🤮", "💩", "🤡"}
	RU_FLAGS_LEN        = len(RU_FLAGS)
	DEFAULT_RU_NAME_LAT = "le pidorashque"
	DEFAULT_RU_NAME_CYR = "підорашка"
)

var (
	Tg       TgValues
	Db       DbValues
	Predator PredatorValues
	Services ServicesValues
	Links    LinksValues
)

type TgMediaItem struct {
	ID int64 `json:"id" req:"true"`
	AH int64 `json:"ah" req:"true"`
}

type TgMedia struct {
	Predator TgMediaItem `json:"predator"`
	Petushok TgMediaItem `json:"petushok"`
	Luhansk  TgMediaItem `json:"luhansk"`
	Golang   TgMediaItem `json:"golang"`
}

type TgCreds struct {
	AppID        int    `json:"app_id" req:"true"`
	AppHash      string `json:"app_hash" req:"true"`
	BotToken     string `json:"bot_token" req:"true"`
	TestBotToken string `json:"test_bot_token"`
}

type TgValues struct {
	UseTestToken bool              `json:"use_test_token"`
	Creds        TgCreds           `json:"creds"`
	SessionPath  string            `json:"session_path" req:"true"`
	OwnerID      int64             `json:"owner_id" req:"true"`
	SupportLink  string            `json:"support_link" req:"true"`
	Media        TgMedia           `json:"media"`
	Autoreplies  map[string]string `json:"autoreplies"`
}

type DbValues struct {
	Path string `json:"path" req:"true"`
}

type PredatorMsgsImportModes struct {
	Unspecified string `json:"unspecified"`
	Skip        string `json:"skip" req:"true"`
	Update      string `json:"update" req:"true"`
	Replace     string `json:"replace" req:"true"`
}

type PredatorMsgs struct {
	Path           string                  `json:"path"`
	ImportMode     string                  `json:"import_mode"`
	AllImportModes PredatorMsgsImportModes `json:"all_import_modes"`
}

type PredatorValues struct {
	Msgs PredatorMsgs `json:"msgs"`
}

type GeminiService struct {
	APIKeys            []string `json:"api_keys" req:"true"`
	ModelsEndpoint     string   `json:"models_endpoint" req:"true"`
	GenerationEndpoint string   `json:"generation_endpoint" req:"true"`
	DefaultModel       string   `json:"default_model" req:"true"`
	BlockNoneString    string   `json:"block_none_string" req:"true"`
}

type WeatherService struct {
	Token string `json:"token" req:"true"`
}

type RusoskotService struct {
	Link             string `json:"link" req:"true"`
	CachePathDaily   string `json:"cache_path_daily" req:"true"`
	CachePathMonthly string `json:"cache_path_monthly" req:"true"`
	EndpointDaily    string `json:"endpoint_daily" req:"true"`
	EndpointMonthly  string `json:"endpoint_monthly" req:"true"`
}

type ServicesValues struct {
	Gemini   GeminiService   `json:"gemini"`
	Weather  WeatherService  `json:"weather"`
	Rusoskot RusoskotService `json:"rusoskot"`
}

type LinksValues struct {
	DuzhocoinsImage          string `json:"duzhocoins_image" req:"true"`
	RandomChatBubbleImage    string `json:"random_chat_bubble_image" req:"true"`
	GeneratedChatBubbleImage string `json:"generated_chat_bubble_image" req:"true"`
}

func Init(initFile InitFile) {
	Tg = initFile.Tg
	Db = initFile.Db
	Predator = initFile.Predator
	Services = initFile.Services
	Links = initFile.Links
}

package ai

type APIKey struct {
	val        string
	dailyUsage int
}

func (k *APIKey) Value() string {
	return k.val
}

func NewApiKey(val string, dailyUsage int) *APIKey {
	return &APIKey{
		val:        val,
		dailyUsage: dailyUsage,
	}
}

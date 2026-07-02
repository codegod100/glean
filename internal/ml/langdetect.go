package ml

type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var knownLanguages = []Language{
	{"ar", "Arabic"},
	{"bn", "Bengali"},
	{"bg", "Bulgarian"},
	{"ca", "Catalan"},
	{"zh", "Chinese"},
	{"cs", "Czech"},
	{"da", "Danish"},
	{"nl", "Dutch"},
	{"en", "English"},
	{"fi", "Finnish"},
	{"fr", "French"},
	{"de", "German"},
	{"el", "Greek"},
	{"he", "Hebrew"},
	{"hi", "Hindi"},
	{"hu", "Hungarian"},
	{"id", "Indonesian"},
	{"it", "Italian"},
	{"ja", "Japanese"},
	{"ko", "Korean"},
	{"ms", "Malay"},
	{"nb", "Norwegian"},
	{"fa", "Persian"},
	{"pl", "Polish"},
	{"pt", "Portuguese"},
	{"ro", "Romanian"},
	{"ru", "Russian"},
	{"sk", "Slovak"},
	{"sl", "Slovenian"},
	{"es", "Spanish"},
	{"sv", "Swedish"},
	{"ta", "Tamil"},
	{"th", "Thai"},
	{"tr", "Turkish"},
	{"uk", "Ukrainian"},
	{"ur", "Urdu"},
	{"vi", "Vietnamese"},
}

var knownSet map[string]bool

func init() {
	knownSet = make(map[string]bool, len(knownLanguages))
	for _, l := range knownLanguages {
		knownSet[l.Code] = true
	}
}

func IsKnownLanguage(code string) bool {
	return knownSet[code]
}

func KnownLanguages() []Language {
	return knownLanguages
}

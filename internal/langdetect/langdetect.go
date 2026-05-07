package langdetect

type Language struct {
	Code string
	Name string
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

func IsKnown(code string) bool {
	for _, l := range knownLanguages {
		if l.Code == code {
			return true
		}
	}
	return false
}

func KnownLanguages() []Language {
	return knownLanguages
}

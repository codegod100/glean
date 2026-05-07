package langdetect

type Language struct {
	Code string
	Name string
}

var knownLanguages = []Language{
	{"en", "English"},
	{"fr", "French"},
	{"de", "German"},
	{"es", "Spanish"},
	{"pt", "Portuguese"},
	{"it", "Italian"},
	{"ru", "Russian"},
	{"ja", "Japanese"},
	{"zh", "Chinese"},
	{"ko", "Korean"},
	{"ar", "Arabic"},
}

func KnownLanguages() []Language {
	return knownLanguages
}

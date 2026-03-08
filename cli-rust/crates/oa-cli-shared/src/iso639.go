// iso639.go — ISO 639-1 语言代码映射表 (S2-6: HIDDEN-4)
//
// 替代 npm 包 iso-639-1 的黑盒行为。
// 纯静态 map，无外部依赖。

package infra

import "strings"

// iso639Codes ISO 639-1 代码 → 英文语言名。
// 涵盖常用 ~50 语言（完整列表约 180 条，按需扩展）。
var iso639Codes = map[string]string{
	"aa": "Afar", "ab": "Abkhaz", "af": "Afrikaans", "am": "Amharic",
	"ar": "Arabic", "az": "Azerbaijani", "ba": "Bashkir", "be": "Belarusian",
	"bg": "Bulgarian", "bn": "Bengali", "bo": "Tibetan", "bs": "Bosnian",
	"ca": "Catalan", "cs": "Czech", "cy": "Welsh", "da": "Danish",
	"de": "German", "el": "Greek", "en": "English", "eo": "Esperanto",
	"es": "Spanish", "et": "Estonian", "eu": "Basque", "fa": "Persian",
	"fi": "Finnish", "fo": "Faroese", "fr": "French", "ga": "Irish",
	"gl": "Galician", "gu": "Gujarati", "ha": "Hausa", "he": "Hebrew",
	"hi": "Hindi", "hr": "Croatian", "hu": "Hungarian", "hy": "Armenian",
	"id": "Indonesian", "is": "Icelandic", "it": "Italian", "ja": "Japanese",
	"ka": "Georgian", "kk": "Kazakh", "km": "Khmer", "kn": "Kannada",
	"ko": "Korean", "ku": "Kurdish", "ky": "Kyrgyz", "la": "Latin",
	"lo": "Lao", "lt": "Lithuanian", "lv": "Latvian", "mk": "Macedonian",
	"ml": "Malayalam", "mn": "Mongolian", "mr": "Marathi", "ms": "Malay",
	"mt": "Maltese", "my": "Burmese", "nb": "Norwegian Bokmål",
	"ne": "Nepali", "nl": "Dutch", "nn": "Norwegian Nynorsk", "no": "Norwegian",
	"pa": "Punjabi", "pl": "Polish", "ps": "Pashto", "pt": "Portuguese",
	"ro": "Romanian", "ru": "Russian", "rw": "Kinyarwanda", "si": "Sinhala",
	"sk": "Slovak", "sl": "Slovenian", "so": "Somali", "sq": "Albanian",
	"sr": "Serbian", "sv": "Swedish", "sw": "Swahili", "ta": "Tamil",
	"te": "Telugu", "tg": "Tajik", "th": "Thai", "tk": "Turkmen",
	"tl": "Tagalog", "tr": "Turkish", "uk": "Ukrainian", "ur": "Urdu",
	"uz": "Uzbek", "vi": "Vietnamese", "yo": "Yoruba",
	"zh": "Chinese", "zu": "Zulu",
}

// iso639Names 英文名（小写）→ ISO 639-1 代码（延迟初始化）。
var iso639Names map[string]string

func initNameMap() {
	if iso639Names != nil {
		return
	}
	iso639Names = make(map[string]string, len(iso639Codes))
	for code, name := range iso639Codes {
		iso639Names[strings.ToLower(name)] = code
	}
}

// ISO639CodeToName 将 ISO 639-1 代码转换为英文语言名。
// 找不到时返回空字符串。
func ISO639CodeToName(code string) string {
	return iso639Codes[strings.ToLower(strings.TrimSpace(code))]
}

// ISO639NameToCode 将英文语言名转换为 ISO 639-1 代码。
// 不区分大小写，找不到时返回空字符串。
func ISO639NameToCode(name string) string {
	initNameMap()
	return iso639Names[strings.ToLower(strings.TrimSpace(name))]
}

// ISO639IsValid 检查是否为有效的 ISO 639-1 代码。
func ISO639IsValid(code string) bool {
	_, ok := iso639Codes[strings.ToLower(strings.TrimSpace(code))]
	return ok
}

package chat

import "strings"

const (
	localeZH = "zh"
	localeEN = "en"
)

func detectTurnLocale(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return localeZH
	}
	if looksMostlyASCII(trimmed) && containsEnglishMarkers(strings.ToLower(trimmed)) {
		return localeEN
	}
	if containsCJK(trimmed) {
		return localeZH
	}
	if looksMostlyASCII(trimmed) {
		return localeEN
	}
	return localeZH
}

func isEnglishLocale(locale string) bool {
	return strings.EqualFold(strings.TrimSpace(locale), localeEN)
}

func containsCJK(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

func looksMostlyASCII(text string) bool {
	if text == "" {
		return false
	}
	ascii := 0
	for _, r := range text {
		if r <= 127 {
			ascii++
		}
	}
	return float64(ascii)/float64(len([]rune(text))) >= 0.8
}

func containsEnglishMarkers(text string) bool {
	markers := []string{" the ", " what ", " where ", " how ", " please ", " can you ", "could you", "would you", "hello", "hi "}
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func localizedText(locale, zh, en string) string {
	if isEnglishLocale(locale) {
		return en
	}
	return zh
}


package githubauth

import "strings"

func validClientID(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 160 {
		return false
	}
	for _, character := range raw {
		isLower := character >= 'a' && character <= 'z'
		isUpper := character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		isAllowedSymbol := strings.ContainsRune(".-_", character)
		if isLower || isUpper || isDigit || isAllowedSymbol {
			continue
		}
		return false
	}
	return true
}

// ValidClientID validates the non-secret identifier returned by GitHub App
// registration or entered through the documented manual fallback.
func ValidClientID(raw string) bool {
	return validClientID(raw)
}

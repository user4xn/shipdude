package util

import "os"

// GetEnv reads an environment variable and falls back to a default value
// when the variable is unset or empty.
func GetEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// GetEnvInt reads an environment variable as an int, falling back to a
// default value when the variable is unset or holds a non-numeric value.
func GetEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	var parsed int
	for _, r := range value {
		if r < '0' || r > '9' {
			return fallback
		}
		parsed = parsed*10 + int(r-'0')
	}
	return parsed
}

package config

import "os"

func DBPath() string {
	return getenv("DB_PATH", "portfolio.db")
}

// BaseCurrency is the reporting currency for newly created accounts.
func BaseCurrency() string {
	return getenv("BASE_CURRENCY", "EUR")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

package config

import (
	"os"
	"path/filepath"
)

func DBPath() string {
	return getenv("DB_PATH", "portfolio.db")
}

// BaseCurrency is the reporting currency for newly created accounts.
func BaseCurrency() string {
	return getenv("BASE_CURRENCY", "EUR")
}

// DataDir holds the files the program works with: IB statements and downloaded
// index data. Its content is not under version control.
func DataDir() string {
	return getenv("DATA_DIR", "data")
}

// StatementsDir is where the .qfx files from IB go.
func StatementsDir() string {
	return filepath.Join(DataDir(), "statements")
}

// CboeDir holds the index history files downloaded from Cboe.
func CboeDir() string {
	return filepath.Join(DataDir(), "cboe")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

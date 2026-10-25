// Package settlements keeps the index levels that cash-settled options expire
// at. They come from the index history files Cboe publishes, which are
// downloaded into a folder and read from there.
package settlements

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"portfolio/models"
)

// Source marks prices taken from Cboe's index history files.
const Source = "Cboe index history"

// Files downloaded by Download: the S&P 500 close (SPX, SPXW and XSP options),
// the opening settlement value of monthly SPX options, and the Russell 2000 close.
var Symbols = []string{"SPX", "SET", "RUT"}

const fileSuffix = "_History.csv"

// the address of the files linked from Cboe's index pages
var downloadURL = "https://cdn.cboe.com/api/global/us_indices/daily_prices/%s" + fileSuffix

const maxFileSize = 32 << 20

// Download fetches the history of every symbol and stores it as SYMBOL_History.csv in dir.
// A file is only replaced when the download is a valid history file.
func Download(client *http.Client, dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	var written []string
	for _, symbol := range Symbols {
		data, err := fetch(client, fmt.Sprintf(downloadURL, symbol))
		if err != nil {
			return written, fmt.Errorf("%s: %w", symbol, err)
		}
		if _, err := ParseHistory(bytes.NewReader(data), symbol, time.Time{}); err != nil {
			return written, fmt.Errorf("%s: %w", symbol, err)
		}

		path := filepath.Join(dir, symbol+fileSuffix)
		if err := writeFile(path, data); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

func fetch(client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "portfolio-go/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Cboe answered %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFileSize))
}

// writeFile replaces path in one step, so an interrupted download cannot leave half a file.
func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// LoadDir reads all SYMBOL_History.csv files in dir. The name of a file gives the
// index its prices are for, so a file in the same format for another index
// (NDX_History.csv, say) is picked up as well. A missing folder holds no prices.
func LoadDir(dir string) ([]models.SettlementPrice, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*"+fileSuffix))
	if err != nil {
		return nil, err
	}

	var prices []models.SettlementPrice
	for _, path := range paths {
		index := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), fileSuffix))

		file, err := os.Open(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		fromFile, err := ParseHistory(file, index, time.Time{})
		file.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		prices = append(prices, fromFile...)
	}
	return prices, nil
}

// ParseHistory reads Cboe's CSV format: "DATE,SYMBOL" or "DATE,OPEN,HIGH,LOW,CLOSE"
// with dates as MM/DD/YYYY. Days before from are skipped.
func ParseHistory(r io.Reader, index string, from time.Time) ([]models.SettlementPrice, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 || len(rows[0]) < 2 || !strings.EqualFold(strings.TrimSpace(rows[0][0]), "date") {
		return nil, fmt.Errorf("not a Cboe history file: unexpected header %v", firstRow(rows))
	}

	column := 1 // a file with one value per day
	for i, name := range rows[0] {
		if strings.EqualFold(strings.TrimSpace(name), "close") {
			column = i
		}
	}

	var prices []models.SettlementPrice
	for _, row := range rows[1:] {
		if len(row) <= column {
			continue
		}
		day, err := time.Parse("01/02/2006", strings.TrimSpace(row[0]))
		if err != nil {
			return nil, fmt.Errorf("bad date %q", row[0])
		}
		if day.Before(from) {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(row[column]), 64)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("bad price %q on %s", row[column], row[0])
		}
		prices = append(prices, models.SettlementPrice{Index: index, Day: day, Price: value, Source: Source})
	}
	return prices, nil
}

func firstRow(rows [][]string) []string {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

package settlements

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseHistorySingleColumn(t *testing.T) {
	data := "DATE,SET\r\n07/09/1998,1162.440000\r\n01/19/2024,4799.620000\r\n09/19/2025,6658.770000\r\n"

	prices, err := ParseHistory(strings.NewReader(data), "SET", time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 2 { // 1998 is before the start
		t.Fatalf("%+v", prices)
	}
	p := prices[0]
	if p.Index != "SET" || p.Price != 4799.62 || p.Day != time.Date(2024, 1, 19, 0, 0, 0, 0, time.UTC) || p.Source != Source {
		t.Errorf("%+v", p)
	}
}

func TestParseHistoryUsesTheCloseColumn(t *testing.T) {
	data := "DATE,OPEN,HIGH,LOW,CLOSE\n01/02/2024,13.2,14.0,12.9,13.6\n"

	prices, err := ParseHistory(strings.NewReader(data), "VIX", time.Time{})
	if err != nil || len(prices) != 1 || prices[0].Price != 13.6 {
		t.Errorf("%+v %v", prices, err)
	}
}

func TestParseHistoryRejectsOtherContent(t *testing.T) {
	for _, data := range []string{
		"",
		`<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`,
		"<!DOCTYPE html><html></html>",
		"DATE,SPX\nnot a date,5\n",
		"DATE,SPX\n01/02/2024,abc\n",
		"DATE,SPX\n01/02/2024,-4\n",
	} {
		if _, err := ParseHistory(strings.NewReader(data), "SPX", time.Time{}); err == nil {
			t.Errorf("%q should be rejected", data)
		}
	}
}

// a server that answers like Cboe's file store: one history file per symbol
func fakeCboe(t *testing.T, answers map[string]string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := answers[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	old := downloadURL
	downloadURL = server.URL + "/%s" + fileSuffix
	t.Cleanup(func() { downloadURL = old })
}

func goodAnswers() map[string]string {
	return map[string]string{
		"SPX_History.csv": "DATE,SPX\n09/19/2025,6664.360000\n",
		"SET_History.csv": "DATE,SET\n09/19/2025,6658.770000\n",
		"RUT_History.csv": "DATE,RUT\n09/19/2025,2400.5000\n",
	}
}

func TestDownloadWritesAndLoadDirReadsThemBack(t *testing.T) {
	fakeCboe(t, goodAnswers())
	dir := filepath.Join(t.TempDir(), "cboe", "nested") // created when missing

	written, err := Download(http.DefaultClient, dir)
	if err != nil || len(written) != 3 {
		t.Fatalf("%v %v", written, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "SPX_History.csv.tmp")); err == nil {
		t.Error("temporary file left behind")
	}

	prices, err := LoadDir(dir)
	if err != nil || len(prices) != 3 {
		t.Fatalf("%+v %v", prices, err)
	}
	byIndex := map[string]float64{}
	for _, p := range prices {
		byIndex[p.Index] = p.Price
	}
	if byIndex["SPX"] != 6664.36 || byIndex["SET"] != 6658.77 || byIndex["RUT"] != 2400.5 {
		t.Errorf("%v", byIndex)
	}
}

func TestDownloadKeepsOldFilesWhenTheAnswerIsNotAHistory(t *testing.T) {
	answers := goodAnswers()
	dir := t.TempDir()
	fakeCboe(t, answers)
	if _, err := Download(http.DefaultClient, dir); err != nil {
		t.Fatal(err)
	}

	// Cboe starts answering with an error page for RUT
	answers["RUT_History.csv"] = `<?xml version="1.0"?><Error><Code>AccessDenied</Code></Error>`
	answers["SPX_History.csv"] = "DATE,SPX\n09/22/2025,6693.750000\n"

	written, err := Download(http.DefaultClient, dir)
	if err == nil || !strings.Contains(err.Error(), "RUT") {
		t.Fatalf("error: %v", err)
	}
	if len(written) != 2 { // SPX and SET were fine and are stored
		t.Errorf("written: %v", written)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "RUT_History.csv"))
	if !strings.Contains(string(data), "2400.5") {
		t.Errorf("the good RUT file was overwritten: %q", data)
	}
}

func TestDownloadReportsHTTPErrors(t *testing.T) {
	fakeCboe(t, map[string]string{"SPX_History.csv": "DATE,SPX\n09/19/2025,1\n"}) // SET is missing: 403

	if _, err := Download(http.DefaultClient, t.TempDir()); err == nil || !strings.Contains(err.Error(), "SET") {
		t.Errorf("error: %v", err)
	}
}

func TestLoadDirPicksUpFilesForOtherIndexes(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "ndx_History.csv"), []byte("DATE,NDX\n09/19/2025,24000\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644)

	prices, err := LoadDir(dir)
	if err != nil || len(prices) != 1 || prices[0].Index != "NDX" {
		t.Errorf("%+v %v", prices, err)
	}
}

func TestLoadDirWithoutFolderHasNoPrices(t *testing.T) {
	prices, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil || len(prices) != 0 {
		t.Errorf("%+v %v", prices, err)
	}
}

func TestLoadDirNamesTheBrokenFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SPX_History.csv"), []byte("garbage"), 0o644)

	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "SPX_History.csv") {
		t.Errorf("%v", err)
	}
}

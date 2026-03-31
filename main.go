package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/extrame/xls"
	_ "github.com/lib/pq"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

var (
	dbUser     = os.Getenv("POSTGRES_USER")
	dbPassword = os.Getenv("POSTGRES_PASSWORD")
	dbHost     = os.Getenv("POSTGRES_HOST")
	dbPort     = os.Getenv("POSTGRES_PORT")
	dbName     = os.Getenv("POSTGRES_DB")
)

const (
	emaeURL = "https://www.indec.gob.ar/ftp/cuadros/economia/sh_emae_mensual_base2004.xls"
)

func databaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName)
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// EMAERow represents one monthly observation of the EMAE indicator.
type EMAERow struct {
	Fecha                       time.Time
	EMAE                        float64
	EMAEVarAnual                *float64
	EMAEDesest                  *float64
	EMAEDesestVarMensual        *float64
	EMAETendenciaCiclo          *float64
	EMAETendenciaCicloVarMensual *float64
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "application/vnd.ms-excel")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("Downloaded %.2f MB → %s\n", float64(n)/(1024*1024), dest)
	return nil
}

// ---------------------------------------------------------------------------
// XLS helpers
// ---------------------------------------------------------------------------

func cellFloat(row *xls.Row, col int) *float64 {
	if col >= int(row.LastCol()) {
		return nil
	}
	s := strings.TrimSpace(row.Col(col))
	if s == "" || s == "--" || s == "///" || s == "…" || s == "s/d" || s == "n/a" {
		return nil
	}
	s = strings.ReplaceAll(s, ",", "")
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// ---------------------------------------------------------------------------
// Parse EMAE sheet
// ---------------------------------------------------------------------------

func parseEMAE(filePath string) ([]EMAERow, error) {
	wb, err := xls.Open(filePath, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("opening XLS: %w", err)
	}

	sheet := wb.GetSheet(0)
	if sheet == nil {
		return nil, fmt.Errorf("sheet 0 not found")
	}

	fmt.Printf("Sheet: %q, rows: %d\n", sheet.Name, int(sheet.MaxRow)+1)

	// The Python code skips 4 rows (skiprows=4) and reads columns C:H (indices 2-7).
	// Row 0-3 are headers; data starts at row 4 (0-indexed).
	// Dates are generated as monthly starting 2004-01-01.
	const (
		startRow = 4   // first data row (0-indexed, after skipping 4 header rows)
		colEMAE  = 2   // column C
		colH     = 7   // column H (last)
	)

	currentDate := time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC)
	var rows []EMAERow

	for i := startRow; i <= int(sheet.MaxRow); i++ {
		row := sheet.Row(i)
		if row == nil {
			currentDate = currentDate.AddDate(0, 1, 0)
			continue
		}

		// EMAE (column C) is required — skip row if missing
		emaeVal := cellFloat(row, colEMAE)
		if emaeVal == nil {
			currentDate = currentDate.AddDate(0, 1, 0)
			continue
		}

		r := EMAERow{
			Fecha:                        currentDate,
			EMAE:                         *emaeVal,
			EMAEVarAnual:                 cellFloat(row, colEMAE+1),
			EMAEDesest:                   cellFloat(row, colEMAE+2),
			EMAEDesestVarMensual:         cellFloat(row, colEMAE+3),
			EMAETendenciaCiclo:           cellFloat(row, colEMAE+4),
			EMAETendenciaCicloVarMensual:  cellFloat(row, colH),
		}
		rows = append(rows, r)
		currentDate = currentDate.AddDate(0, 1, 0)
	}

	fmt.Printf("Parsed %d EMAE observations\n", len(rows))
	return rows, nil
}

// ---------------------------------------------------------------------------
// Database insert — COPY (bulk, truncates first)
// ---------------------------------------------------------------------------

func insertCopy(db *sql.DB, rows []EMAERow, truncate bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if truncate {
		if _, err := tx.Exec("TRUNCATE TABLE emae RESTART IDENTITY"); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
	}

	stmt, err := tx.Prepare(`
		COPY emae (fecha, emae, emae_var_anual, emae_desest,
		           emae_desest_var_mensual, emae_tendencia_ciclo,
		           emae_tendencia_ciclo_var_mensual)
		FROM STDIN`)
	if err != nil {
		// COPY via lib/pq is not supported with Prepare; fall back to row inserts
		return insertRows(tx, rows)
	}
	defer stmt.Close()

	for i, r := range rows {
		_, err := stmt.Exec(
			r.Fecha,
			r.EMAE,
			nullFloat(r.EMAEVarAnual),
			nullFloat(r.EMAEDesest),
			nullFloat(r.EMAEDesestVarMensual),
			nullFloat(r.EMAETendenciaCiclo),
			nullFloat(r.EMAETendenciaCicloVarMensual),
		)
		if err != nil {
			return fmt.Errorf("row %d: %w", i, err)
		}
		if (i+1)%1000 == 0 {
			fmt.Printf("  COPY progress: %d / %d\n", i+1, len(rows))
		}
	}

	if _, err := stmt.Exec(); err != nil {
		return fmt.Errorf("COPY finish: %w", err)
	}

	return tx.Commit()
}

// insertRows is used as fallback when COPY is not available.
func insertRows(tx *sql.Tx, rows []EMAERow) error {
	stmt, err := tx.Prepare(`
		INSERT INTO emae (fecha, emae, emae_var_anual, emae_desest,
		                  emae_desest_var_mensual, emae_tendencia_ciclo,
		                  emae_tendencia_ciclo_var_mensual)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, r := range rows {
		_, err := stmt.Exec(
			r.Fecha,
			r.EMAE,
			nullFloat(r.EMAEVarAnual),
			nullFloat(r.EMAEDesest),
			nullFloat(r.EMAEDesestVarMensual),
			nullFloat(r.EMAETendenciaCiclo),
			nullFloat(r.EMAETendenciaCicloVarMensual),
		)
		if err != nil {
			return fmt.Errorf("row %d: %w", i, err)
		}
		if (i+1)%1000 == 0 {
			fmt.Printf("  INSERT progress: %d / %d\n", i+1, len(rows))
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Database insert — UPSERT
// ---------------------------------------------------------------------------

func insertUpsert(db *sql.DB, rows []EMAERow, truncate bool) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if truncate {
		if _, err := tx.Exec("TRUNCATE TABLE emae RESTART IDENTITY"); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
	}

	stmt, err := tx.Prepare(`
		INSERT INTO emae (fecha, emae, emae_var_anual, emae_desest,
		                  emae_desest_var_mensual, emae_tendencia_ciclo,
		                  emae_tendencia_ciclo_var_mensual)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (fecha)
		DO UPDATE SET
			emae                             = EXCLUDED.emae,
			emae_var_anual                   = EXCLUDED.emae_var_anual,
			emae_desest                      = EXCLUDED.emae_desest,
			emae_desest_var_mensual          = EXCLUDED.emae_desest_var_mensual,
			emae_tendencia_ciclo             = EXCLUDED.emae_tendencia_ciclo,
			emae_tendencia_ciclo_var_mensual = EXCLUDED.emae_tendencia_ciclo_var_mensual,
			ingested_at                      = NOW()`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, r := range rows {
		_, err := stmt.Exec(
			r.Fecha,
			r.EMAE,
			nullFloat(r.EMAEVarAnual),
			nullFloat(r.EMAEDesest),
			nullFloat(r.EMAEDesestVarMensual),
			nullFloat(r.EMAETendenciaCiclo),
			nullFloat(r.EMAETendenciaCicloVarMensual),
		)
		if err != nil {
			return fmt.Errorf("row %d: %w", i, err)
		}
		if (i+1)%1000 == 0 {
			fmt.Printf("  UPSERT progress: %d / %d\n", i+1, len(rows))
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nullFloat(v *float64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func checkEnvVars() error {
	missing := []string{}
	for _, k := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_HOST", "POSTGRES_PORT", "POSTGRES_DB"} {
		if os.Getenv(k) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	filePath := flag.String("file", "", "Path to local XLS file (skips download)")
	truncate := flag.Bool("truncate", false, "Truncate table before inserting")
	upsert := flag.Bool("upsert", false, "Use UPSERT instead of COPY")
	flag.Parse()

	if err := checkEnvVars(); err != nil {
		log.Fatal(err)
	}

	start := time.Now()

	// --- Download or use local file ---
	var xlsPath string
	if *filePath != "" {
		xlsPath = *filePath
		fmt.Printf("Using local file: %s\n", xlsPath)
	} else {
		tmp, err := os.MkdirTemp("", "emae_downloader_")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)

		xlsPath = filepath.Join(tmp, "emae.xls")
		if err := downloadFile(emaeURL, xlsPath); err != nil {
			log.Fatalf("Download failed: %v", err)
		}
	}

	// --- Parse ---
	rows, err := parseEMAE(xlsPath)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}
	if len(rows) == 0 {
		fmt.Println("No rows parsed. Exiting.")
		return
	}

	fmt.Printf("Date range: %s → %s\n",
		rows[0].Fecha.Format("2006-01-02"),
		rows[len(rows)-1].Fecha.Format("2006-01-02"))

	// --- Database ---
	db, err := sql.Open("postgres", databaseURL())
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}
	fmt.Println("Connected to PostgreSQL")

	if *upsert {
		fmt.Println("Insert mode: UPSERT")
		if err := insertUpsert(db, rows, *truncate); err != nil {
			log.Fatalf("UPSERT failed: %v", err)
		}
	} else {
		fmt.Println("Insert mode: COPY")
		if err := insertCopy(db, rows, *truncate); err != nil {
			log.Fatalf("COPY failed: %v", err)
		}
	}

	fmt.Printf("Done: %d rows inserted in %s\n", len(rows), time.Since(start).Round(time.Millisecond))
}

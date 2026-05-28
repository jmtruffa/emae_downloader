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
	emaeURL          = "https://www.indec.gob.ar/ftp/cuadros/economia/sh_emae_mensual_base2004.xls"
	emaeActividadURL = "https://www.indec.gob.ar/ftp/cuadros/economia/sh_emae_actividad_base2004.xls"
)

func databaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName)
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

type EMAERow struct {
	Fecha                        time.Time
	EMAE                         float64
	EMAEVarAnual                 *float64
	EMAEDesest                   *float64
	EMAEDesestVarMensual         *float64
	EMAETendenciaCiclo           *float64
	EMAETendenciaCicloVarMensual *float64
}

type EMAEActividadRow struct {
	Fecha   time.Time
	Valores [emaeActividadSectorCount]*float64
}

const emaeActividadSectorCount = 16

var emaeActividadColumns = [emaeActividadSectorCount]string{
	"agricultura_ganaderia_caza_silvicultura",
	"pesca",
	"explotacion_minas_canteras",
	"industria_manufacturera",
	"electricidad_gas_agua",
	"construccion",
	"comercio_mayorista_minorista_reparaciones",
	"hoteles_restaurantes",
	"transporte_comunicaciones",
	"intermediacion_financiera",
	"actividades_inmobiliarias_empresariales_alquiler",
	"administracion_publica_defensa_seguridad_social",
	"ensenanza",
	"servicios_sociales_salud",
	"otras_actividades_servicios_comunitarios_sociales_personales",
	"impuestos_netos_subsidios",
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

// safeRow wraps sheet.Row() with panic recovery — the extrame/xls library
// can panic on certain rows with corrupted or missing data.
func safeRow(sheet *xls.WorkSheet, idx int) (row *xls.Row) {
	defer func() {
		if r := recover(); r != nil {
			row = nil
		}
	}()
	return sheet.Row(idx)
}

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

	const (
		startRow = 4
		colEMAE  = 2
		colH     = 7
	)

	currentDate := time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC)
	var rows []EMAERow

	// Advance currentDate only after a row is successfully consumed. Header /
	// blank rows between startRow and the first data row would otherwise shift
	// the entire series forward by one month.
	for i := startRow; i <= int(sheet.MaxRow); i++ {
		row := safeRow(sheet, i)
		if row == nil {
			continue
		}

		emaeVal := cellFloat(row, colEMAE)
		if emaeVal == nil {
			continue
		}

		r := EMAERow{
			Fecha:                        currentDate,
			EMAE:                         *emaeVal,
			EMAEVarAnual:                 cellFloat(row, colEMAE+1),
			EMAEDesest:                   cellFloat(row, colEMAE+2),
			EMAEDesestVarMensual:         cellFloat(row, colEMAE+3),
			EMAETendenciaCiclo:           cellFloat(row, colEMAE+4),
			EMAETendenciaCicloVarMensual: cellFloat(row, colH),
		}
		rows = append(rows, r)
		currentDate = currentDate.AddDate(0, 1, 0)
	}

	fmt.Printf("Parsed %d EMAE observations\n", len(rows))
	return rows, nil
}

// ---------------------------------------------------------------------------
// Parse EMAE activity sheets
// ---------------------------------------------------------------------------

func parseSpanishMonth(s string) (time.Month, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enero":
		return time.January, true
	case "febrero":
		return time.February, true
	case "marzo":
		return time.March, true
	case "abril":
		return time.April, true
	case "mayo":
		return time.May, true
	case "junio":
		return time.June, true
	case "julio":
		return time.July, true
	case "agosto":
		return time.August, true
	case "septiembre":
		return time.September, true
	case "octubre":
		return time.October, true
	case "noviembre":
		return time.November, true
	case "diciembre":
		return time.December, true
	default:
		return 0, false
	}
}

func parseYearCell(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var year int
	if _, err := fmt.Sscanf(s, "%d", &year); err != nil {
		return 0, false
	}
	if year < 1900 || year > 2200 {
		return 0, false
	}
	return year, true
}

func parseActividadSheet(sheet *xls.WorkSheet) (map[time.Time][emaeActividadSectorCount]*float64, []time.Time) {
	values := map[time.Time][emaeActividadSectorCount]*float64{}
	var order []time.Time
	currentYear := 0

	for i := 0; i <= int(sheet.MaxRow); i++ {
		row := safeRow(sheet, i)
		if row == nil {
			continue
		}

		if year, ok := parseYearCell(row.Col(0)); ok {
			currentYear = year
		}

		month, ok := parseSpanishMonth(row.Col(1))
		if !ok || currentYear == 0 {
			continue
		}

		var rowValues [emaeActividadSectorCount]*float64
		hasValue := false
		for c := 0; c < emaeActividadSectorCount; c++ {
			v := cellFloat(row, c+2)
			rowValues[c] = v
			if v != nil {
				hasValue = true
			}
		}
		if !hasValue {
			continue
		}

		fecha := time.Date(currentYear, month, 1, 0, 0, 0, 0, time.UTC)
		if _, exists := values[fecha]; !exists {
			order = append(order, fecha)
		}
		values[fecha] = rowValues
	}

	return values, order
}

func parseEMAEActividad(filePath string) ([]EMAEActividadRow, error) {
	wb, err := xls.Open(filePath, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("opening activity XLS: %w", err)
	}

	indicesSheet := wb.GetSheet(0)
	if indicesSheet == nil {
		return nil, fmt.Errorf("activity indices sheet 0 not found")
	}

	fmt.Printf("Activity sheet: %q, rows: %d\n", indicesSheet.Name, int(indicesSheet.MaxRow)+1)

	indices, order := parseActividadSheet(indicesSheet)

	rows := make([]EMAEActividadRow, 0, len(order))
	for _, fecha := range order {
		r := EMAEActividadRow{
			Fecha:   fecha,
			Valores: indices[fecha],
		}
		rows = append(rows, r)
	}

	fmt.Printf("Parsed %d EMAE activity observations\n", len(rows))
	return rows, nil
}

// ---------------------------------------------------------------------------
// Database
// ---------------------------------------------------------------------------

// lastIngestedMaxFecha returns the max(fecha) across the rows belonging to the
// most recent ingested_at snapshot. Returns zero time if the table is empty.
func lastIngestedMaxFecha(db *sql.DB) (time.Time, error) {
	var t sql.NullTime
	err := db.QueryRow(`
		SELECT MAX(fecha)
		FROM emae
		WHERE ingested_at = (SELECT MAX(ingested_at) FROM emae)`).Scan(&t)
	if err != nil {
		return time.Time{}, err
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, nil
}

func insertSnapshot(db *sql.DB, rows []EMAERow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ingestedAt := time.Now().UTC()

	stmt, err := tx.Prepare(`
		INSERT INTO emae (fecha, emae, emae_var_anual, emae_desest,
		                  emae_desest_var_mensual, emae_tendencia_ciclo,
		                  emae_tendencia_ciclo_var_mensual, ingested_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`)
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
			ingestedAt,
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

// lastActividadIngestedMaxFecha returns the max(fecha) across the rows
// belonging to the most recent emae_actividad snapshot.
func lastActividadIngestedMaxFecha(db *sql.DB) (time.Time, error) {
	var t sql.NullTime
	err := db.QueryRow(`
		SELECT MAX(fecha)
		FROM emae_actividad
		WHERE ingested_at = (SELECT MAX(ingested_at) FROM emae_actividad)`).Scan(&t)
	if err != nil {
		return time.Time{}, err
	}
	if !t.Valid {
		return time.Time{}, nil
	}
	return t.Time, nil
}

func insertActividadSnapshot(db *sql.DB, rows []EMAEActividadRow) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	ingestedAt := time.Now().UTC()

	columns := []string{"fecha"}
	columns = append(columns, emaeActividadColumns[:]...)
	columns = append(columns, "ingested_at")

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	stmt, err := tx.Prepare(fmt.Sprintf(`
		INSERT INTO emae_actividad (%s)
		VALUES (%s)`, strings.Join(columns, ", "), strings.Join(placeholders, ", ")))
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i, r := range rows {
		args := make([]any, 0, len(columns))
		args = append(args, r.Fecha)
		for _, v := range r.Valores {
			args = append(args, nullFloat(v))
		}
		args = append(args, ingestedAt)

		if _, err := stmt.Exec(args...); err != nil {
			return fmt.Errorf("activity row %d: %w", i, err)
		}
		if (i+1)%1000 == 0 {
			fmt.Printf("  INSERT activity progress: %d / %d\n", i+1, len(rows))
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nullFloat(v *float64) any {
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
	actividadFilePath := flag.String("actividad-file", "", "Path to local activity XLS file (skips activity download)")
	force := flag.Bool("force", false, "Insert snapshot even if max(fecha) did not advance")
	flag.Parse()

	if err := checkEnvVars(); err != nil {
		log.Fatal(err)
	}

	start := time.Now()

	tmp, err := os.MkdirTemp("", "emae_downloader_")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)

	var xlsPath string
	if *filePath != "" {
		xlsPath = *filePath
		fmt.Printf("Using local file: %s\n", xlsPath)
	} else {
		xlsPath = filepath.Join(tmp, "emae.xls")
		if err := downloadFile(emaeURL, xlsPath); err != nil {
			log.Fatalf("Download failed: %v", err)
		}
	}

	var actividadXLSPath string
	if *actividadFilePath != "" {
		actividadXLSPath = *actividadFilePath
		fmt.Printf("Using local activity file: %s\n", actividadXLSPath)
	} else {
		actividadXLSPath = filepath.Join(tmp, "emae_actividad.xls")
		if err := downloadFile(emaeActividadURL, actividadXLSPath); err != nil {
			log.Fatalf("Activity download failed: %v", err)
		}
	}

	rows, err := parseEMAE(xlsPath)
	if err != nil {
		log.Fatalf("Parse failed: %v", err)
	}
	if len(rows) == 0 {
		log.Fatal("No EMAE rows parsed.")
	}

	parsedMax := rows[len(rows)-1].Fecha
	fmt.Printf("Date range: %s → %s\n",
		rows[0].Fecha.Format("2006-01-02"),
		parsedMax.Format("2006-01-02"))

	actividadRows, err := parseEMAEActividad(actividadXLSPath)
	if err != nil {
		log.Fatalf("Activity parse failed: %v", err)
	}
	if len(actividadRows) == 0 {
		log.Fatal("No EMAE activity rows parsed.")
	}

	actividadParsedMax := actividadRows[len(actividadRows)-1].Fecha
	fmt.Printf("Activity date range: %s → %s\n",
		actividadRows[0].Fecha.Format("2006-01-02"),
		actividadParsedMax.Format("2006-01-02"))

	db, err := sql.Open("postgres", databaseURL())
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("DB ping: %v", err)
	}
	fmt.Println("Connected to PostgreSQL")

	lastMax, err := lastIngestedMaxFecha(db)
	if err != nil {
		log.Fatalf("Querying last snapshot: %v", err)
	}

	if !lastMax.IsZero() {
		fmt.Printf("Last snapshot max(fecha): %s\n", lastMax.Format("2006-01-02"))
		if !parsedMax.After(lastMax) && !*force {
			fmt.Println("No new fecha vs last snapshot — skipping insert. Use -force to override.")
		} else if err := insertSnapshot(db, rows); err != nil {
			log.Fatalf("Insert failed: %v", err)
		}
	} else {
		fmt.Println("Table emae is empty — inserting initial snapshot.")
		if err := insertSnapshot(db, rows); err != nil {
			log.Fatalf("Insert failed: %v", err)
		}
	}

	actividadLastMax, err := lastActividadIngestedMaxFecha(db)
	if err != nil {
		log.Fatalf("Querying last activity snapshot: %v", err)
	}

	if !actividadLastMax.IsZero() {
		fmt.Printf("Last activity snapshot max(fecha): %s\n", actividadLastMax.Format("2006-01-02"))
		if !actividadParsedMax.After(actividadLastMax) && !*force {
			fmt.Println("No new activity fecha vs last snapshot — skipping insert. Use -force to override.")
		} else if err := insertActividadSnapshot(db, actividadRows); err != nil {
			log.Fatalf("Activity insert failed: %v", err)
		}
	} else {
		fmt.Println("Table emae_actividad is empty — inserting initial activity snapshot.")
		if err := insertActividadSnapshot(db, actividadRows); err != nil {
			log.Fatalf("Activity insert failed: %v", err)
		}
	}

	fmt.Printf("Done in %s\n", time.Since(start).Round(time.Millisecond))
}

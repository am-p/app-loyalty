// Command fixtures creates an isolated, synthetic-only dataset for the local
// stress-remediation suite. It never calls the application HTTP API.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const (
	databaseName       = "puntazo_load"
	databasePort       = 55437
	expectedSchema     = "0034"
	brandCount         = 100
	customerCount      = 10_000
	movementsPerCard   = 10
	fixturePasswordLen = 32
)

type fixtureDocument struct {
	SchemaVersion int             `json:"schema_version"`
	Identities    []fixturePerson `json:"identities"`
}

type fixturePerson struct {
	VU          int    `json:"vu"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	AccountType string `json:"account_type"`
}

type seededCustomer struct {
	ID    int64
	Email string
}

type seededBrand struct {
	ID         int64
	Name       string
	OwnerID    int64
	BranchID   int64
	ProgramID  int64
	BranchName string
}

type populationCounts struct {
	Brands     int `json:"brands"`
	Staff      int `json:"brand_staff"`
	Branches   int `json:"branches"`
	Programs   int `json:"programs"`
	Customers  int `json:"customers"`
	Cards      int `json:"cards"`
	Movements  int `json:"movements"`
	FixtureVUs int `json:"fixture_vus"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "stress fixture seed rejected: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("STRESS_LOCAL_FIXTURES") != "true" {
		return errors.New("set STRESS_LOCAL_FIXTURES=true to acknowledge a local synthetic fixture operation")
	}

	flags := flag.NewFlagSet("stress-fixtures", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("out", "", "absolute path outside the repository for the private VU credential fixture")
	vus := flags.Int("vus", 3, "number of independent suite credentials to create (3 or 5)")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if *vus != 3 && *vus != 5 {
		return errors.New("-vus must be 3 or 5")
	}
	if *output == "" || !filepath.IsAbs(*output) {
		return errors.New("-out must be an absolute path outside the repository")
	}
	outputPath, err := filepath.Abs(*output)
	if err != nil {
		return errors.New("invalid fixture output path")
	}
	if pathWithinRepository(outputPath) {
		return errors.New("credential fixture output must be outside the repository")
	}
	if _, err := os.Lstat(outputPath); err == nil {
		return errors.New("fixture output already exists; choose a new private path")
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("could not inspect fixture output path")
	}

	dsn := os.Getenv("STRESS_DATABASE_URL")
	if dsn == "" {
		return errors.New("set STRESS_DATABASE_URL to the dedicated local puntazo_load database")
	}
	connConfig, err := validateDatabaseTarget(dsn)
	if err != nil {
		return err
	}

	password, err := randomPassword()
	if err != nil {
		return errors.New("could not generate a synthetic fixture password")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("could not prepare the shared synthetic fixture password hash")
	}
	if cost, err := bcrypt.Cost(passwordHash); err != nil || cost != bcrypt.DefaultCost {
		return errors.New("synthetic password hash cost did not match the default safe cost")
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0700); err != nil {
		return errors.New("could not prepare private fixture output directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return errors.New("invalid local database connection configuration")
	}
	poolConfig.ConnConfig = connConfig
	poolConfig.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return errors.New("could not connect to the dedicated local fixture database")
	}
	defer pool.Close()

	fixture := fixtureDocument{SchemaVersion: 1, Identities: make([]fixturePerson, 0, *vus)}
	counts := populationCounts{Brands: brandCount, Staff: brandCount, Branches: brandCount, Programs: brandCount, Customers: customerCount, Cards: customerCount, Movements: customerCount * movementsPerCard, FixtureVUs: *vus}
	var committed bool
	var fixtureCreated bool
	defer func() {
		if fixtureCreated && !committed {
			_ = os.Remove(outputPath)
		}
	}()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return errors.New("could not start the local seed transaction")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := prepareEmptyDatabase(ctx, tx); err != nil {
		return err
	}
	if err := seedPopulation(ctx, tx, passwordHash, password, *vus, &fixture); err != nil {
		return err
	}
	if err := verifyPopulation(ctx, tx); err != nil {
		return err
	}

	fixtureFile, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("could not create the private credential fixture; no database changes were committed")
	}
	fixtureCreated = true
	if err := fixtureFile.Chmod(0600); err != nil {
		_ = fixtureFile.Close()
		return errors.New("could not restrict credential fixture permissions")
	}
	encoder := json.NewEncoder(fixtureFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(fixture); err != nil {
		_ = fixtureFile.Close()
		return errors.New("could not write the private credential fixture")
	}
	if err := fixtureFile.Sync(); err != nil {
		_ = fixtureFile.Close()
		return errors.New("could not sync the private credential fixture")
	}
	if err := fixtureFile.Close(); err != nil {
		return errors.New("could not close the private credential fixture")
	}

	if err := tx.Commit(ctx); err != nil {
		return errors.New("local seed transaction failed; the credential fixture was removed")
	}
	committed = true
	result := struct {
		Database string           `json:"database"`
		Schema   string           `json:"schema"`
		Fixture  string           `json:"fixture_file"`
		Counts   populationCounts `json:"counts"`
	}{Database: databaseName, Schema: expectedSchema, Fixture: outputPath, Counts: counts}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return errors.New("dataset committed, but stdout report could not be written")
	}
	return nil
}

func validateDatabaseTarget(dsn string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid local database connection configuration")
	}
	host := strings.ToLower(strings.Trim(config.Host, "[]"))
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, errors.New("fixture seeding only permits localhost, 127.0.0.1, or ::1")
	}
	if config.Database != databaseName {
		return nil, errors.New("fixture seeding only permits the dedicated puntazo_load database")
	}
	if config.Port != databasePort {
		return nil, errors.New("fixture seeding only permits the local PostgreSQL port 55437")
	}
	return config, nil
}

func pathWithinRepository(path string) bool {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return true
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(source), "../../../"))
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return true
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func randomPassword() (string, error) {
	bytes := make([]byte, fixturePasswordLen)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func prepareEmptyDatabase(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return errors.New("could not set the local fixture lock timeout")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(1947362871)`); err != nil {
		return errors.New("could not acquire the fixture seeding lock")
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE public.schema_migrations IN ACCESS EXCLUSIVE MODE`); err != nil {
		return errors.New("could not lock migration history; migrate the dedicated database before seeding")
	}
	var applied int
	var minVersion, maxVersion string
	var distinct int
	if err := tx.QueryRow(ctx, `SELECT count(*),min(btrim(version)),max(btrim(version)),count(DISTINCT btrim(version)) FROM schema_migrations`).Scan(&applied, &minVersion, &maxVersion, &distinct); err != nil {
		return errors.New("schema_migrations is missing; migrate the dedicated database before seeding")
	}
	if applied != 34 || distinct != 34 || minVersion != "0001" || maxVersion != expectedSchema {
		return fmt.Errorf("expected the complete schema %s migration history; found %d applied migrations", expectedSchema, applied)
	}

	tableRows, err := tx.Query(ctx, `SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname='public' ORDER BY tablename`)
	if err != nil {
		return errors.New("could not enumerate local fixture tables")
	}
	var tables []string
	for tableRows.Next() {
		var table string
		if err := tableRows.Scan(&table); err != nil {
			tableRows.Close()
			return errors.New("could not read local fixture table list")
		}
		tables = append(tables, table)
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return errors.New("could not enumerate local fixture tables")
	}
	tableRows.Close()
	if len(tables) == 0 {
		return errors.New("public schema has no migrated tables")
	}
	for _, table := range tables {
		if _, err := tx.Exec(ctx, `LOCK TABLE `+pgx.Identifier{"public", table}.Sanitize()+` IN ACCESS EXCLUSIVE MODE`); err != nil {
			return errors.New("could not lock the local schema; stop local writers and retry")
		}
	}
	for _, table := range tables {
		if table == "schema_migrations" {
			continue
		}
		var rows int64
		query := `SELECT count(*) FROM ` + pgx.Identifier{"public", table}.Sanitize()
		if err := tx.QueryRow(ctx, query).Scan(&rows); err != nil {
			return errors.New("could not check the local database is empty")
		}
		if rows != 0 {
			return fmt.Errorf("table %s is populated; refusing to modify this database", table)
		}
	}
	return verifyRequiredColumns(ctx, tx)
}

func verifyRequiredColumns(ctx context.Context, tx pgx.Tx) error {
	required := [][2]string{
		{"usuarios", "email"}, {"usuarios", "password_hash"}, {"usuarios", "nombre"}, {"usuarios", "apellido"}, {"usuarios", "tipo_cuenta"}, {"usuarios", "qr_hash"}, {"usuarios", "email_verified_at"}, {"usuarios", "auth_version"},
		{"marcas", "nombre"}, {"sucursales", "marca_id"}, {"sucursales", "principal"}, {"programas_fidelidad", "sellos_por_acumulacion"}, {"programas_fidelidad", "nombre_unidad"},
		{"membresias_marca", "usuario_id"}, {"membresias_sucursales", "marca_id"}, {"tarjetas", "saldo_puntos"},
		{"historial_movimientos", "programa_tipo"}, {"historial_movimientos", "marca_nombre_snapshot"}, {"historial_movimientos", "sucursal_nombre_snapshot"}, {"historial_movimientos", "programa_id_snapshot"},
	}
	for _, column := range required {
		var found bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, column[0], column[1]).Scan(&found); err != nil || !found {
			return fmt.Errorf("required modern-schema column %s.%s is unavailable", column[0], column[1])
		}
	}
	return nil
}

func seedPopulation(ctx context.Context, tx pgx.Tx, passwordHash []byte, password string, vus int, fixture *fixtureDocument) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	brands := make([]seededBrand, brandCount)
	for i := range brands {
		number := i + 1
		brand := &brands[i]
		brand.Name = fmt.Sprintf("Stress Fixture Brand %03d", number)
		brand.BranchName = fmt.Sprintf("Stress Fixture Branch %03d", number)
		if err := tx.QueryRow(ctx, `INSERT INTO marcas(nombre) VALUES($1) RETURNING id`, brand.Name).Scan(&brand.ID); err != nil {
			return errors.New("could not create synthetic brands")
		}
		ownerEmail := fmt.Sprintf("load-owner-%03d@stress.puntazo.test", number)
		ownerName := fmt.Sprintf("Stress Owner %03d", number)
		if err := tx.QueryRow(ctx, `INSERT INTO usuarios(email,password_hash,nombre,apellido,tipo_cuenta,email_verified_at,auth_version) VALUES($1,$2,$3,'Fixture','PERSONAL_MARCA',$4,1) RETURNING id`, ownerEmail, string(passwordHash), ownerName, now).Scan(&brand.OwnerID); err != nil {
			return errors.New("could not create synthetic brand staff")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO sucursales(marca_id,nombre,principal) VALUES($1,$2,true) RETURNING id`, brand.ID, brand.BranchName).Scan(&brand.BranchID); err != nil {
			return errors.New("could not create synthetic branches")
		}
		if err := tx.QueryRow(ctx, `INSERT INTO programas_fidelidad(marca_id,tipo,sellos_por_acumulacion,nombre_unidad) VALUES($1,'PUNTOS',NULL,'punto') RETURNING id`, brand.ID).Scan(&brand.ProgramID); err != nil {
			return errors.New("could not create synthetic points programs")
		}
		var membershipID int64
		if err := tx.QueryRow(ctx, `INSERT INTO membresias_marca(usuario_id,marca_id,rol) VALUES($1,$2,'PROPIETARIO') RETURNING id`, brand.OwnerID, brand.ID).Scan(&membershipID); err != nil {
			return errors.New("could not create synthetic brand memberships")
		}
		if _, err := tx.Exec(ctx, `INSERT INTO membresias_sucursales(membresia_id,sucursal_id,marca_id) VALUES($1,$2,$3)`, membershipID, brand.BranchID, brand.ID); err != nil {
			return errors.New("could not attach synthetic staff to branches")
		}
	}

	customerRows := make([][]any, 0, customerCount)
	emails := make([]string, customerCount)
	for i := 0; i < customerCount; i++ {
		number := i + 1
		email := fmt.Sprintf("load-customer-%05d@stress.puntazo.test", number)
		emails[i] = email
		qrHash := make([]byte, 32)
		if _, err := rand.Read(qrHash); err != nil {
			return errors.New("could not prepare synthetic customer identity hashes")
		}
		customerRows = append(customerRows, []any{
			email, string(passwordHash), fmt.Sprintf("Stress Customer %05d", number), "Fixture",
			"CLIENTE_FINAL", qrHash, now, 1,
		})
		if number <= vus {
			fixture.Identities = append(fixture.Identities, fixturePerson{
				VU: number, Email: email, Password: password, AccountType: "CLIENTE_FINAL",
			})
		}
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"usuarios"}, []string{"email", "password_hash", "nombre", "apellido", "tipo_cuenta", "qr_hash", "email_verified_at", "auth_version"}, pgx.CopyFromRows(customerRows)); err != nil {
		return errors.New("could not insert synthetic customers")
	}

	customers, err := loadCustomers(ctx, tx, emails)
	if err != nil || len(customers) != customerCount {
		return errors.New("could not map inserted synthetic customers")
	}
	cardRows := make([][]any, 0, customerCount)
	for i, customer := range customers {
		brand := brands[i%brandCount]
		cardRows = append(cardRows, []any{customer.ID, brand.ID, int64(100), true})
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tarjetas"}, []string{"usuario_id", "marca_id", "saldo_puntos", "activo"}, pgx.CopyFromRows(cardRows)); err != nil {
		return errors.New("could not insert synthetic loyalty cards")
	}
	cards, err := loadCards(ctx, tx, emails)
	if err != nil || len(cards) != customerCount {
		return errors.New("could not map inserted synthetic cards")
	}

	movementRows := make([][]any, 0, customerCount*movementsPerCard)
	for i, card := range cards {
		brand := brands[i%brandCount]
		for movement := 0; movement < movementsPerCard; movement++ {
			before := int64(movement * 10)
			movementRows = append(movementRows, []any{
				uuid.New(), card.ID, brand.ID, brand.BranchID, brand.OwnerID, nil,
				"ACUMULACION", "CREDITO", int64(10), before, before + 10,
				nil, nil, now.Add(time.Duration(movement-9) * time.Hour), "PUNTOS", nil,
				brand.Name, brand.BranchName, brand.ProgramID,
			})
		}
	}
	columns := []string{
		"operation_id", "tarjeta_id", "marca_id", "sucursal_id", "usuario_operador_id", "beneficio_id",
		"operacion", "sentido", "cantidad", "saldo_anterior", "saldo_posterior",
		"beneficio_nombre_snapshot", "beneficio_requisito_snapshot", "occurred_at", "programa_tipo",
		"beneficio_requisito_puntos_snapshot", "marca_nombre_snapshot", "sucursal_nombre_snapshot", "programa_id_snapshot",
	}
	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"historial_movimientos"}, columns, pgx.CopyFromRows(movementRows)); err != nil {
		return errors.New("could not insert synthetic point movements")
	}
	return nil
}

func loadCustomers(ctx context.Context, tx pgx.Tx, emails []string) ([]seededCustomer, error) {
	rows, err := tx.Query(ctx, `SELECT id,email::text FROM usuarios WHERE email::text=ANY($1::text[]) ORDER BY email::text`, emails)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	customers := make([]seededCustomer, 0, len(emails))
	for rows.Next() {
		var customer seededCustomer
		if err := rows.Scan(&customer.ID, &customer.Email); err != nil {
			return nil, err
		}
		customers = append(customers, customer)
	}
	return customers, rows.Err()
}

type seededCard struct {
	ID     int64
	UserID int64
	Brand  int64
}

func loadCards(ctx context.Context, tx pgx.Tx, emails []string) ([]seededCard, error) {
	rows, err := tx.Query(ctx, `SELECT t.id,t.usuario_id,t.marca_id FROM tarjetas t JOIN usuarios u ON u.id=t.usuario_id WHERE u.email::text=ANY($1::text[]) ORDER BY u.email::text`, emails)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cards := make([]seededCard, 0, len(emails))
	for rows.Next() {
		var card seededCard
		if err := rows.Scan(&card.ID, &card.UserID, &card.Brand); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func verifyPopulation(ctx context.Context, tx pgx.Tx) error {
	expected := map[string]int64{
		"marcas":                brandCount,
		"usuarios":              brandCount + customerCount,
		"membresias_marca":      brandCount,
		"sucursales":            brandCount,
		"programas_fidelidad":   brandCount,
		"membresias_sucursales": brandCount,
		"tarjetas":              customerCount,
		"historial_movimientos": customerCount * movementsPerCard,
	}
	for table, want := range expected {
		var got int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{"public", table}.Sanitize()).Scan(&got); err != nil || got != want {
			return fmt.Errorf("synthetic table count did not match expectation for %s", table)
		}
	}
	var cards, badLedgers int64
	if err := tx.QueryRow(ctx, `
		WITH per_card AS (
			SELECT h.tarjeta_id,count(*) AS movements,min(h.saldo_anterior) AS first_balance,
			       max(h.saldo_posterior) AS final_balance,sum(h.cantidad) AS credited,
			       bool_and(h.operacion='ACUMULACION' AND h.sentido='CREDITO' AND h.cantidad=10
			                AND h.saldo_posterior=h.saldo_anterior+h.cantidad) AS rows_valid
			FROM historial_movimientos h GROUP BY h.tarjeta_id
		)
		SELECT count(*),count(*) FILTER(WHERE movements<>10 OR first_balance<>0 OR final_balance<>100 OR credited<>100 OR NOT rows_valid)
		FROM per_card`).Scan(&cards, &badLedgers); err != nil || cards != customerCount || badLedgers != 0 {
		return errors.New("per-card movement ledger did not reconcile")
	}
	var discontinuities int64
	if err := tx.QueryRow(ctx, `
		WITH sequence AS (
			SELECT saldo_anterior,lag(saldo_posterior,1,0) OVER(PARTITION BY tarjeta_id ORDER BY occurred_at,id) AS previous_balance
			FROM historial_movimientos
		)
		SELECT count(*) FROM sequence WHERE saldo_anterior<>previous_balance`).Scan(&discontinuities); err != nil || discontinuities != 0 {
		return errors.New("movement balance chain did not reconcile")
	}
	var mismatchedCards int64
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM tarjetas t
		LEFT JOIN (SELECT tarjeta_id,max(saldo_posterior) AS ledger_balance FROM historial_movimientos GROUP BY tarjeta_id) h ON h.tarjeta_id=t.id
		WHERE t.saldo_puntos<>100 OR h.ledger_balance IS DISTINCT FROM t.saldo_puntos`).Scan(&mismatchedCards); err != nil || mismatchedCards != 0 {
		return errors.New("card point balances did not match their full movement ledgers")
	}
	return nil
}

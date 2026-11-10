package migrations

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// TestSchema owns only its nine tables in an initially empty, dedicated *_test
// database. The account needs database-wide DDL/DML privileges, not CREATE DATABASE.
func TestSchema(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_DSN to an empty dedicated MySQL 8.4 *_test database")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid TEST_DATABASE_DSN (credentials omitted)")
	}
	if len(cfg.DBName) > 64 || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*_test$`).MatchString(cfg.DBName) {
		t.Fatal("TEST_DATABASE_DSN database must be an ASCII identifier ending _test")
	}
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.MultiStatements = false
	cfg.Timeout = 10 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	if cfg.Params == nil {
		cfg.Params = make(map[string]string)
	}
	cfg.Params["time_zone"] = "'+00:00'"
	cfg.Params["foreign_key_checks"] = "1"
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal("invalid MySQL test configuration (credentials omitted)")
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("connect to dedicated test database: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	// A pinned connection holds the advisory lock until after table cleanup.
	lockHash := sha256.Sum256([]byte(strings.ToLower(cfg.DBName)))
	lockName := fmt.Sprintf("koperasi-schema:%x", lockHash[:16])
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", lockName).Scan(&locked); err != nil {
		t.Fatalf("acquire schema test lock: %v", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		t.Fatal("another schema test holds this database lock")
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		var released sql.NullInt64
		if err := conn.QueryRowContext(cleanupCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
			t.Errorf("release schema test lock: %v", err)
		} else if !released.Valid || released.Int64 != 1 {
			t.Error("schema test lock was not held until cleanup")
		}
	})

	var objects int
	err = conn.QueryRowContext(ctx, `SELECT
        (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?) +
        (SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = ?) +
        (SELECT COUNT(*) FROM information_schema.events WHERE event_schema = ?) +
        (SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema = ?)`,
		cfg.DBName, cfg.DBName, cfg.DBName, cfg.DBName).Scan(&objects)
	if err != nil {
		t.Fatalf("inspect test database before DDL: %v", err)
	}
	if objects != 0 {
		t.Fatal("refusing to mutate a nonempty test database; no tables will be removed")
	}

	tables := []string{"users", "sessions", "members", "savings", "loans", "installments", "payments", "payment_allocations", "loan_settlements"}
	created := make(map[string]bool, len(tables))
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		for i := len(tables) - 1; i >= 0; i-- {
			if created[tables[i]] {
				if _, err := conn.ExecContext(cleanupCtx, "DROP TABLE `"+tables[i]+"`"); err != nil {
					t.Errorf("remove owned test table %s: %v", tables[i], err)
				}
			}
		}
	})
	for _, file := range []string{"001_schema.sql", "002_constraints_indexes.sql"} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		// shortcut: only standalone SQL and line comments; use a SQL parser if
		// migrations gain stored programs or semicolons inside quoted values.
		var lines []string
		for _, line := range strings.Split(string(source), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				lines = append(lines, line)
			}
		}
		for _, statement := range strings.Split(strings.Join(lines, "\n"), ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := conn.ExecContext(ctx, statement); err != nil {
				t.Fatalf("apply %s: %v", file, err)
			}
			for _, table := range tables {
				if strings.HasPrefix(statement, "CREATE TABLE "+table+" (") {
					created[table] = true
				}
			}
		}
	}

	exec := func(t *testing.T, query string, args ...any) int64 {
		t.Helper()
		result, err := conn.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("expected statement to succeed: %v", err)
		}
		id, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	reject := func(t *testing.T, code uint16, query string, args ...any) {
		t.Helper()
		_, err := conn.ExecContext(ctx, query, args...)
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != code {
			t.Fatalf("expected MySQL error %d, got %v", code, err)
		}
	}
	var sequence int
	key := func() string {
		sequence++
		return fmt.Sprintf("%032x", sequence)
	}
	userID := exec(t, `INSERT INTO users (name, username, password_hash, is_active)
        VALUES ('Schema test', 'schema-test', 'unused-test-hash', 1)`)
	member := func(number string) int64 {
		return exec(t, `INSERT INTO members
            (member_number, name, address, phone, join_date, status, created_by)
            VALUES (?, 'Schema test', 'Test address', '0800000000', '2026-01-01', 'ACTIVE', ?)`, number, userID)
	}
	memberID := member("SCHEMA-1")
	guardMemberID := member("SCHEMA-2")
	activeLoanID := exec(t, `INSERT INTO loans
        (member_id, principal, tenor_months, application_date, status, created_by, approved_by, disbursed_by, disbursement_date)
        VALUES (?, 600000, 6, '2026-01-01', 'ACTIVE', ?, ?, ?, '2026-01-02')`, memberID, userID, userID, userID)
	installmentID := exec(t, `INSERT INTO installments
        (loan_id, installment_number, due_date, principal_due, interest_due)
        VALUES (?, 1, '2026-02-10', 100000, 6000)`, activeLoanID)
	paymentID := exec(t, `INSERT INTO payments
        (loan_id, user_id, payment_type, payment_date, amount, request_key)
        VALUES (?, ?, 'EARLY_SETTLEMENT', '2026-02-05', 100000, ?)`, activeLoanID, userID, key())

	t.Run("engine and UTC business dates", func(t *testing.T) {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
            WHERE table_schema = ? AND engine = 'InnoDB' AND table_collation LIKE 'utf8mb4%'`, cfg.DBName).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 9 {
			t.Fatalf("expected nine utf8mb4 InnoDB tables, got %d", count)
		}
		var zone string
		var date time.Time
		if err := conn.QueryRowContext(ctx, "SELECT @@session.time_zone, join_date FROM members WHERE id = ?", memberID).Scan(&zone, &date); err != nil {
			t.Fatal(err)
		}
		if zone != "+00:00" || date.Format("2006-01-02") != "2026-01-01" || date.Location() != time.UTC {
			t.Fatalf("unexpected session timezone or DATE: %s %v", zone, date)
		}
	})

	savingsSQL := `INSERT INTO savings
        (member_id, user_id, saving_type, transaction_type, amount, period_month, transaction_date, request_key)
        VALUES (?, ?, ?, ?, ?, ?, '2026-02-01', ?)`
	t.Run("nominal enum and period boundaries", func(t *testing.T) {
		cases := []struct {
			name, saving, transaction string
			amount                    int64
			period                    any
		}{
			{"zero", "SUKARELA", "SETOR", 0, nil},
			{"negative", "SUKARELA", "SETOR", -1, nil},
			{"principal", "POKOK", "SETOR", 499999, nil},
			{"mandatory", "WAJIB", "SETOR", 99999, "2026-01-01"},
			{"missing period", "WAJIB", "SETOR", 100000, nil},
			{"period not first day", "WAJIB", "SETOR", 100000, "2026-01-02"},
			{"unrelated period", "SUKARELA", "SETOR", 1, "2026-01-01"},
			{"principal withdrawal", "POKOK", "TARIK", 1, nil},
			{"mandatory withdrawal", "WAJIB", "TARIK", 1, nil},
			{"unknown type", "OTHER", "SETOR", 1, nil},
			{"case sensitive type", "pokok", "SETOR", 500000, nil},
			{"unknown transaction", "SUKARELA", "OTHER", 1, nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				reject(t, 3819, savingsSQL, memberID, userID, tc.saving, tc.transaction, tc.amount, tc.period, key())
			})
		}
		reject(t, 3819, "UPDATE users SET is_active = 2 WHERE id = ?", userID)
		for _, tenor := range []int{5, 25} {
			reject(t, 3819, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
                VALUES (?, 1, ?, '2026-01-01', 'PENDING', ?)`, guardMemberID, tenor, userID)
		}
		for _, principal := range []int{0, -1} {
			reject(t, 3819, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
                VALUES (?, ?, 6, '2026-01-01', 'PENDING', ?)`, guardMemberID, principal, userID)
		}
		for _, tenor := range []int{6, 24} {
			exec(t, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by, rejected_by, rejection_reason)
                VALUES (?, 1, ?, '2026-01-01', 'REJECTED', ?, ?, 'Test rejection')`, guardMemberID, tenor, userID, userID)
		}
	})

	t.Run("member null safe exit state", func(t *testing.T) {
		for _, update := range []string{
			"status = 'EXITED'",
			"status = 'EXITED', exit_date = '2026-02-01'",
			fmt.Sprintf("status = 'EXITED', exit_user_id = %d", userID),
			fmt.Sprintf("status = 'EXITED', exit_date = '2025-12-31', exit_user_id = %d", userID),
			"exit_date = '2026-02-01'",
			fmt.Sprintf("exit_user_id = %d", userID),
			"status = 'OTHER'",
		} {
			reject(t, 3819, "UPDATE members SET "+update+" WHERE id = ?", memberID)
		}
		exec(t, "UPDATE members SET status = 'EXITED', exit_date = '2026-02-01', exit_user_id = ? WHERE id = ?", userID, memberID)
		exec(t, "UPDATE members SET status = 'ACTIVE', exit_date = NULL, exit_user_id = NULL WHERE id = ?", memberID)
	})

	t.Run("loan lifecycle and nullable historical guard", func(t *testing.T) {
		loanID := exec(t, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
            VALUES (?, 1, 6, '2026-01-01', 'PENDING', ?)`, guardMemberID, userID)
		duplicate := func() {
			reject(t, 1062, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
                VALUES (?, 1, 6, '2026-01-01', 'PENDING', ?)`, guardMemberID, userID)
		}
		duplicate()
		for _, update := range []string{
			"status = 'APPROVED'",
			"status = 'ACTIVE'",
			"status = 'SETTLED'",
			"status = 'REJECTED'",
			fmt.Sprintf("status = 'REJECTED', rejected_by = %d, rejection_reason = ''", userID),
			fmt.Sprintf("status = 'REJECTED', rejected_by = %d, rejection_reason = '   '", userID),
			"rejection_reason = 'Not rejected'",
		} {
			reject(t, 3819, "UPDATE loans SET "+update+" WHERE id = ?", loanID)
		}
		exec(t, "UPDATE loans SET status = 'APPROVED', approved_by = ? WHERE id = ?", userID, loanID)
		duplicate()
		reject(t, 3819, `UPDATE loans SET status = 'ACTIVE', disbursed_by = ?, disbursement_date = '2025-12-31' WHERE id = ?`, userID, loanID)
		exec(t, "UPDATE loans SET status = 'ACTIVE', disbursed_by = ?, disbursement_date = '2026-01-02' WHERE id = ?", userID, loanID)
		duplicate()
		reject(t, 3819, "UPDATE loans SET status = 'SETTLED', settled_on = '2026-01-01' WHERE id = ?", loanID)
		exec(t, "UPDATE loans SET status = 'SETTLED', settled_on = '2026-02-01' WHERE id = ?", loanID)
		nextLoanID := exec(t, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
            VALUES (?, 1, 6, '2026-02-02', 'PENDING', ?)`, guardMemberID, userID)
		exec(t, "UPDATE loans SET status = 'APPROVED', approved_by = ? WHERE id = ?", userID, nextLoanID)
		exec(t, "UPDATE loans SET status = 'REJECTED', rejected_by = ?, rejection_reason = 'Cancelled before disbursement' WHERE id = ?", userID, nextLoanID)
		exec(t, `INSERT INTO loans (member_id, principal, tenor_months, application_date, status, created_by)
            VALUES (?, 1, 6, '2026-02-03', 'PENDING', ?)`, guardMemberID, userID)
		var histories int
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM loans WHERE member_id = ? AND open_loan_guard IS NULL", guardMemberID).Scan(&histories); err != nil {
			t.Fatal(err)
		}
		if histories < 2 {
			t.Fatal("closed loan histories did not retain nullable guards")
		}
	})

	t.Run("savings conditional uniqueness and free savings", func(t *testing.T) {
		exec(t, savingsSQL, memberID, userID, "POKOK", "SETOR", 500000, nil, key())
		reject(t, 1062, savingsSQL, memberID, userID, "POKOK", "SETOR", 500000, nil, key())
		exec(t, savingsSQL, memberID, userID, "WAJIB", "SETOR", 100000, "2026-01-01", key())
		reject(t, 1062, savingsSQL, memberID, userID, "WAJIB", "SETOR", 100000, "2026-01-01", key())
		exec(t, savingsSQL, memberID, userID, "WAJIB", "SETOR", 100000, "2026-02-01", key())
		for _, savingType := range []string{"POKOK", "WAJIB", "SUKARELA"} {
			exec(t, savingsSQL, memberID, userID, savingType, "PENGEMBALIAN_KELUAR", 1, nil, key())
			reject(t, 1062, savingsSQL, memberID, userID, savingType, "PENGEMBALIAN_KELUAR", 1, nil, key())
		}
		for range 2 {
			exec(t, savingsSQL, memberID, userID, "SUKARELA", "SETOR", 1, nil, key())
			exec(t, savingsSQL, memberID, userID, "SUKARELA", "TARIK", 1, nil, key())
		}
		lower, upper := strings.Repeat("a", 32), strings.Repeat("A", 32)
		exec(t, savingsSQL, memberID, userID, "SUKARELA", "SETOR", 1, nil, lower)
		exec(t, savingsSQL, memberID, userID, "SUKARELA", "SETOR", 1, nil, upper)
		reject(t, 1062, savingsSQL, memberID, userID, "SUKARELA", "SETOR", 1, nil, lower)
	})

	t.Run("installment boundaries and waiver dates", func(t *testing.T) {
		for _, update := range []string{
			"installment_number = 0", "installment_number = 25", "due_date = '2026-02-11'",
			"principal_due = -1", "interest_due = -1", "interest_waived = -1",
			"interest_waived = 6001, waived_on = '2026-02-05'", "interest_waived = 1",
			"waived_on = '2026-02-05'",
		} {
			reject(t, 3819, "UPDATE installments SET "+update+" WHERE id = ?", installmentID)
		}
		reject(t, 1062, `INSERT INTO installments (loan_id, installment_number, due_date, principal_due, interest_due)
            VALUES (?, 1, '2026-03-10', 0, 0)`, activeLoanID)
		exec(t, "UPDATE installments SET interest_waived = 6000, waived_on = '2026-02-05' WHERE id = ?", installmentID)
		exec(t, "UPDATE installments SET interest_waived = 0, waived_on = NULL WHERE id = ?", installmentID)
		exec(t, `INSERT INTO installments (loan_id, installment_number, due_date, principal_due, interest_due)
            VALUES (?, 24, '2028-01-10', 0, 0)`, activeLoanID)
	})

	allocationSQL := "INSERT INTO payment_allocations (payment_id, installment_id, component_type, amount) VALUES (?, ?, ?, ?)"
	t.Run("allocation constraints payment and settlement amounts", func(t *testing.T) {
		for _, component := range []string{"DENDA", "JASA", "POKOK"} {
			reject(t, 3819, allocationSQL, paymentID, nil, component, 1)
			exec(t, allocationSQL, paymentID, installmentID, component, 1)
		}
		reject(t, 3819, allocationSQL, paymentID, installmentID, "ADMIN", 1)
		reject(t, 3819, allocationSQL, paymentID, nil, "OTHER", 1)
		reject(t, 3819, allocationSQL, paymentID, installmentID, "JASA", 0)
		reject(t, 3819, allocationSQL, paymentID, installmentID, "JASA", -1)
		exec(t, allocationSQL, paymentID, nil, "ADMIN", 1)
		reject(t, 1452, allocationSQL, paymentID, 9223372036854775807, "POKOK", 1)
		for _, update := range []string{"amount = 0", "amount = -1", "payment_type = 'OTHER'"} {
			reject(t, 3819, "UPDATE payments SET "+update+" WHERE id = ?", paymentID)
		}
		reject(t, 1062, `INSERT INTO payments (loan_id, user_id, payment_type, payment_date, amount, request_key)
            SELECT loan_id, user_id, payment_type, payment_date, amount, request_key FROM payments WHERE id = ?`, paymentID)
		settlementID := exec(t, `INSERT INTO loan_settlements
            (loan_id, payment_id, current_installment_id, arrears_amount, remaining_principal, current_interest, administration_fee, waived_interest)
            VALUES (?, ?, ?, 0, 0, 0, 0, 0)`, activeLoanID, paymentID, installmentID)
		for _, column := range []string{"arrears_amount", "remaining_principal", "current_interest", "administration_fee", "waived_interest"} {
			reject(t, 3819, "UPDATE loan_settlements SET "+column+" = -1 WHERE id = ?", settlementID)
		}
	})

	t.Run("foreign keys restrict financial history deletion", func(t *testing.T) {
		for _, target := range []struct {
			table string
			id    int64
		}{{"users", userID}, {"members", memberID}, {"loans", activeLoanID}, {"installments", installmentID}, {"payments", paymentID}} {
			reject(t, 1451, "DELETE FROM "+target.table+" WHERE id = ?", target.id)
		}
	})

	t.Run("failed allocation rolls back payment details and closure", func(t *testing.T) {
		requestKey := key()
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		result, err := tx.ExecContext(ctx, `INSERT INTO payments
            (loan_id, user_id, payment_type, payment_date, amount, request_key)
            VALUES (?, ?, 'REGULAR', '2026-02-10', 2, ?)`, activeLoanID, userID, requestKey)
		if err != nil {
			t.Fatal(err)
		}
		failedPaymentID, err := result.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, allocationSQL, failedPaymentID, installmentID, "POKOK", 1); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE installments SET closed_on = '2026-02-10' WHERE id = ?", installmentID); err != nil {
			t.Fatal(err)
		}
		_, err = tx.ExecContext(ctx, allocationSQL, failedPaymentID, nil, "POKOK", 1)
		var mysqlErr *mysql.MySQLError
		if !errors.As(err, &mysqlErr) || mysqlErr.Number != 3819 {
			t.Fatalf("expected allocation CHECK failure, got %v", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		var paymentCount, allocationCount int
		var closedOn sql.NullTime
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM payments WHERE request_key = ?", requestKey).Scan(&paymentCount); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM payment_allocations WHERE payment_id = ?", failedPaymentID).Scan(&allocationCount); err != nil {
			t.Fatal(err)
		}
		if err := conn.QueryRowContext(ctx, "SELECT closed_on FROM installments WHERE id = ?", installmentID).Scan(&closedOn); err != nil {
			t.Fatal(err)
		}
		if paymentCount != 0 || allocationCount != 0 || closedOn.Valid {
			t.Fatal("failed transaction left a payment, allocation, or installment closure")
		}
	})
}

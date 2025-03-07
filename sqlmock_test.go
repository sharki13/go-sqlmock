package sqlmock

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func cancelOrder(db *sql.DB, orderID int) error {
	tx, _ := db.Begin()
	_, _ = tx.Query("SELECT * FROM orders {0} FOR UPDATE", orderID)
	err := tx.Rollback()
	if err != nil {
		return err
	}
	return nil
}

func Example() {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	// columns to be used for result
	columns := []string{"id", "status"}
	// expect transaction begin
	mock.ExpectBegin()
	// expect query to fetch order, match it with regexp
	mock.ExpectQuery("SELECT (.+) FROM orders (.+) FOR UPDATE").
		WithArgs(1).
		WillReturnRows(NewRows(columns).AddRow(1, 1))
	// expect transaction rollback, since order status is "cancelled"
	mock.ExpectRollback()

	// run the cancel order function
	someOrderID := 1
	// call a function which executes expected database operations
	err = cancelOrder(db, someOrderID)
	if err != nil {
		fmt.Printf("unexpected error: %s", err)
		return
	}

	// ensure all expectations have been met
	if err = mock.ExpectationsWereMet(); err != nil {
		fmt.Printf("unmet expectation error: %s", err)
	}
	// Output:
}

func TestIssue14EscapeSQL(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO mytable\\(a, b\\)").
		WithArgs("A", "B").
		WillReturnResult(NewResult(1, 1))

	_, err = db.Exec("INSERT INTO mytable(a, b) VALUES (?, ?)", "A", "B")
	if err != nil {
		t.Errorf("error '%s' was not expected, while inserting a row", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

// test the case when db is not triggered and expectations
// are not asserted on close
func TestIssue4(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectQuery("some sql query which will not be called").
		WillReturnRows(NewRows([]string{"id"}))

	if err := mock.ExpectationsWereMet(); err == nil {
		t.Errorf("was expecting an error since query was not triggered")
	}
}

func TestMockQuery(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rs := NewRows([]string{"id", "title"}).FromCSVString("5,hello world")

	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs)

	rows, err := db.Query("SELECT (.+) FROM articles WHERE id = ?", 5)
	if err != nil {
		t.Errorf("error '%s' was not expected while retrieving mock rows", err)
	}

	defer func() {
		if er := rows.Close(); er != nil {
			t.Error("unexpected error while trying to close rows")
		}
	}()

	if !rows.Next() {
		t.Error("it must have had one row as result, but got empty result set instead")
	}

	var id int
	var title string

	err = rows.Scan(&id, &title)
	if err != nil {
		t.Errorf("error '%s' was not expected while trying to scan row", err)
	}

	if id != 5 {
		t.Errorf("expected mocked id to be 5, but got %d instead", id)
	}

	if title != "hello world" {
		t.Errorf("expected mocked title to be 'hello world', but got '%s' instead", title)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestMockQueryTypes(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	columns := []string{"id", "timestamp", "sold"}

	timestamp := time.Now()
	rs := NewRows(columns)
	rs.AddRow(5, timestamp, true)

	mock.ExpectQuery("SELECT (.+) FROM sales WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs)

	rows, err := db.Query("SELECT (.+) FROM sales WHERE id = ?", 5)
	if err != nil {
		t.Errorf("error '%s' was not expected while retrieving mock rows", err)
	}
	defer func() {
		if er := rows.Close(); er != nil {
			t.Error("unexpected error while trying to close rows")
		}
	}()
	if !rows.Next() {
		t.Error("it must have had one row as result, but got empty result set instead")
	}

	var id int
	var time time.Time
	var sold bool

	err = rows.Scan(&id, &time, &sold)
	if err != nil {
		t.Errorf("error '%s' was not expected while trying to scan row", err)
	}

	if id != 5 {
		t.Errorf("expected mocked id to be 5, but got %d instead", id)
	}

	if time != timestamp {
		t.Errorf("expected mocked time to be %s, but got '%s' instead", timestamp, time)
	}

	if sold != true {
		t.Errorf("expected mocked boolean to be true, but got %v instead", sold)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestTransactionExpectations(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// begin and commit
	mock.ExpectBegin()
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Errorf("an error '%s' was not expected when beginning a transaction", err)
	}

	err = tx.Commit()
	if err != nil {
		t.Errorf("an error '%s' was not expected when committing a transaction", err)
	}

	// begin and rollback
	mock.ExpectBegin()
	mock.ExpectRollback()

	tx, err = db.Begin()
	if err != nil {
		t.Errorf("an error '%s' was not expected when beginning a transaction", err)
	}

	err = tx.Rollback()
	if err != nil {
		t.Errorf("an error '%s' was not expected when rolling back a transaction", err)
	}

	// begin with an error
	mock.ExpectBegin().WillReturnError(fmt.Errorf("some err"))

	tx, err = db.Begin()
	if err == nil {
		t.Error("an error was expected when beginning a transaction, but got none")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestPrepareExpectations(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectPrepare("SELECT (.+) FROM articles WHERE id = ?")

	stmt, err := db.Prepare("SELECT (.+) FROM articles WHERE id = ?")
	if err != nil {
		t.Errorf("error '%s' was not expected while creating a prepared statement", err)
	}
	if stmt == nil {
		t.Errorf("stmt was expected while creating a prepared statement")
	}

	// expect something else, w/o ExpectPrepare()
	var id int
	var title string
	rs := NewRows([]string{"id", "title"}).FromCSVString("5,hello world")

	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs)

	err = stmt.QueryRow(5).Scan(&id, &title)
	if err != nil {
		t.Errorf("error '%s' was not expected while retrieving mock rows", err)
	}

	mock.ExpectPrepare("SELECT (.+) FROM articles WHERE id = ?").
		WillReturnError(fmt.Errorf("Some DB error occurred"))

	stmt, err = db.Prepare("SELECT id FROM articles WHERE id = ?")
	if err == nil {
		t.Error("error was expected while creating a prepared statement")
	}
	if stmt != nil {
		t.Errorf("stmt was not expected while creating a prepared statement returning error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestPreparedQueryExecutions(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectPrepare("SELECT (.+) FROM articles WHERE id = ?")

	rs1 := NewRows([]string{"id", "title"}).FromCSVString("5,hello world")
	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs1)

	rs2 := NewRows([]string{"id", "title"}).FromCSVString("2,whoop")
	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(2).
		WillReturnRows(rs2)

	stmt, err := db.Prepare("SELECT id, title FROM articles WHERE id = ?")
	if err != nil {
		t.Errorf("error '%s' was not expected while creating a prepared statement", err)
	}

	var id int
	var title string
	err = stmt.QueryRow(5).Scan(&id, &title)
	if err != nil {
		t.Errorf("error '%s' was not expected querying row from statement and scanning", err)
	}

	if id != 5 {
		t.Errorf("expected mocked id to be 5, but got %d instead", id)
	}

	if title != "hello world" {
		t.Errorf("expected mocked title to be 'hello world', but got '%s' instead", title)
	}

	err = stmt.QueryRow(2).Scan(&id, &title)
	if err != nil {
		t.Errorf("error '%s' was not expected querying row from statement and scanning", err)
	}

	if id != 2 {
		t.Errorf("expected mocked id to be 2, but got %d instead", id)
	}

	if title != "whoop" {
		t.Errorf("expected mocked title to be 'whoop', but got '%s' instead", title)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestUnorderedPreparedQueryExecutions(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.MatchExpectationsInOrder(false)

	mock.ExpectPrepare("SELECT (.+) FROM articles WHERE id = ?").
		ExpectQuery().
		WithArgs(5).
		WillReturnRows(NewRows([]string{"id", "title"}).FromCSVString("5,The quick brown fox"))
	mock.ExpectPrepare("SELECT (.+) FROM authors WHERE id = ?").
		ExpectQuery().
		WithArgs(1).
		WillReturnRows(NewRows([]string{"id", "title"}).FromCSVString("1,Betty B."))

	var id int
	var name string

	stmt, err := db.Prepare("SELECT id, name FROM authors WHERE id = ?")
	if err != nil {
		t.Errorf("error '%s' was not expected while creating a prepared statement", err)
	}

	err = stmt.QueryRow(1).Scan(&id, &name)
	if err != nil {
		t.Errorf("error '%s' was not expected querying row from statement and scanning", err)
	}

	if name != "Betty B." {
		t.Errorf("expected mocked name to be 'Betty B.', but got '%s' instead", name)
	}
}

func TestUnexpectedOperations(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectPrepare("SELECT (.+) FROM articles WHERE id = ?")
	stmt, err := db.Prepare("SELECT id, title FROM articles WHERE id = ?")
	if err != nil {
		t.Errorf("error '%s' was not expected while creating a prepared statement", err)
	}

	var id int
	var title string

	err = stmt.QueryRow(5).Scan(&id, &title)
	if err == nil {
		t.Error("error was expected querying row, since there was no such expectation")
	}

	mock.ExpectRollback()

	if err := mock.ExpectationsWereMet(); err == nil {
		t.Errorf("was expecting an error since query was not triggered")
	}
}

func TestWrongExpectations(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()

	rs1 := NewRows([]string{"id", "title"}).FromCSVString("5,hello world")
	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs1)

	mock.ExpectCommit().WillReturnError(fmt.Errorf("deadlock occurred"))
	mock.ExpectRollback() // won't be triggered

	var id int
	var title string

	err = db.QueryRow("SELECT id, title FROM articles WHERE id = ? FOR UPDATE", 5).Scan(&id, &title)
	if err == nil {
		t.Error("error was expected while querying row, since there begin transaction expectation is not fulfilled")
	}

	// lets go around and start transaction
	tx, err := db.Begin()
	if err != nil {
		t.Errorf("an error '%s' was not expected when beginning a transaction", err)
	}

	err = db.QueryRow("SELECT id, title FROM articles WHERE id = ? FOR UPDATE", 5).Scan(&id, &title)
	if err != nil {
		t.Errorf("error '%s' was not expected while querying row, since transaction was started", err)
	}

	err = tx.Commit()
	if err == nil {
		t.Error("a deadlock error was expected when committing a transaction", err)
	}

	if err := mock.ExpectationsWereMet(); err == nil {
		t.Errorf("was expecting an error since query was not triggered")
	}
}

func TestExecExpectations(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	result := NewResult(1, 1)
	mock.ExpectExec("^INSERT INTO articles").
		WithArgs("hello").
		WillReturnResult(result)

	res, err := db.Exec("INSERT INTO articles (title) VALUES (?)", "hello")
	if err != nil {
		t.Errorf("error '%s' was not expected, while inserting a row", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		t.Errorf("error '%s' was not expected, while getting a last insert id", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		t.Errorf("error '%s' was not expected, while getting affected rows", err)
	}

	if id != 1 {
		t.Errorf("expected last insert id to be 1, but got %d instead", id)
	}

	if affected != 1 {
		t.Errorf("expected affected rows to be 1, but got %d instead", affected)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestRowBuilderAndNilTypes(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rs := NewRows([]string{"id", "active", "created", "status"}).
		AddRow(1, true, time.Now(), 5).
		AddRow(2, false, nil, nil)

	mock.ExpectQuery("SELECT (.+) FROM sales").WillReturnRows(rs)

	rows, err := db.Query("SELECT * FROM sales")
	if err != nil {
		t.Errorf("error '%s' was not expected while retrieving mock rows", err)
	}
	defer func() {
		if er := rows.Close(); er != nil {
			t.Error("Unexpected error while trying to close rows")
		}
	}()

	// NullTime and NullInt are used from stubs_test.go
	var (
		id      int
		active  bool
		created NullTime
		status  NullInt
	)

	if !rows.Next() {
		t.Error("it must have had row in rows, but got empty result set instead")
	}

	err = rows.Scan(&id, &active, &created, &status)
	if err != nil {
		t.Errorf("error '%s' was not expected while trying to scan row", err)
	}

	if id != 1 {
		t.Errorf("expected mocked id to be 1, but got %d instead", id)
	}

	if !active {
		t.Errorf("expected 'active' to be 'true', but got '%v' instead", active)
	}

	if !created.Valid {
		t.Errorf("expected 'created' to be valid, but it %+v is not", created)
	}

	if !status.Valid {
		t.Errorf("expected 'status' to be valid, but it %+v is not", status)
	}

	if status.Integer != 5 {
		t.Errorf("expected 'status' to be '5', but got '%d'", status.Integer)
	}

	// test second row
	if !rows.Next() {
		t.Error("it must have had row in rows, but got empty result set instead")
	}

	err = rows.Scan(&id, &active, &created, &status)
	if err != nil {
		t.Errorf("error '%s' was not expected while trying to scan row", err)
	}

	if id != 2 {
		t.Errorf("expected mocked id to be 2, but got %d instead", id)
	}

	if active {
		t.Errorf("expected 'active' to be 'false', but got '%v' instead", active)
	}

	if created.Valid {
		t.Errorf("expected 'created' to be invalid, but it %+v is not", created)
	}

	if status.Valid {
		t.Errorf("expected 'status' to be invalid, but it %+v is not", status)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestArgumentReflectValueTypeError(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rs := NewRows([]string{"id"}).AddRow(1)

	mock.ExpectQuery("SELECT (.+) FROM sales").WithArgs(5.5).WillReturnRows(rs)

	_, err = db.Query("SELECT * FROM sales WHERE x = ?", 5)
	if err == nil {
		t.Error("expected error, but got none")
	}
}

func TestGoroutineExecutionWithUnorderedExpectationMatching(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// note this line is important for unordered expectation matching
	mock.MatchExpectationsInOrder(false)

	result := NewResult(1, 1)

	mock.ExpectExec("^UPDATE one").WithArgs("one").WillReturnResult(result)
	mock.ExpectExec("^UPDATE two").WithArgs("one", "two").WillReturnResult(result)
	mock.ExpectExec("^UPDATE three").WithArgs("one", "two", "three").WillReturnResult(result)

	var wg sync.WaitGroup
	queries := map[string][]interface{}{
		"one":   {"one"},
		"two":   {"one", "two"},
		"three": {"one", "two", "three"},
	}

	wg.Add(len(queries))
	for table, args := range queries {
		go func(tbl string, a []interface{}) {
			if _, err := db.Exec("UPDATE "+tbl, a...); err != nil {
				t.Errorf("error was not expected: %s", err)
			}
			wg.Done()
		}(table, args)
	}

	wg.Wait()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func ExampleSqlmock_goroutines() {
	db, mock, err := New()
	if err != nil {
		fmt.Println("failed to open sqlmock database:", err)
	}
	defer db.Close()

	// note this line is important for unordered expectation matching
	mock.MatchExpectationsInOrder(false)

	result := NewResult(1, 1)

	mock.ExpectExec("^UPDATE one").WithArgs("one").WillReturnResult(result)
	mock.ExpectExec("^UPDATE two").WithArgs("one", "two").WillReturnResult(result)
	mock.ExpectExec("^UPDATE three").WithArgs("one", "two", "three").WillReturnResult(result)

	var wg sync.WaitGroup
	queries := map[string][]interface{}{
		"one":   {"one"},
		"two":   {"one", "two"},
		"three": {"one", "two", "three"},
	}

	wg.Add(len(queries))
	for table, args := range queries {
		go func(tbl string, a []interface{}) {
			if _, err := db.Exec("UPDATE "+tbl, a...); err != nil {
				fmt.Println("error was not expected:", err)
			}
			wg.Done()
		}(table, args)
	}

	wg.Wait()

	if err := mock.ExpectationsWereMet(); err != nil {
		fmt.Println("there were unfulfilled expectations:", err)
	}
	// Output:
}

// False Positive - passes despite mismatched Exec
// see #37 issue
func TestRunExecsWithOrderedShouldNotMeetAllExpectations(t *testing.T) {
	db, dbmock, _ := New()
	dbmock.ExpectExec("THE FIRST EXEC")
	dbmock.ExpectExec("THE SECOND EXEC")

	_, _ = db.Exec("THE FIRST EXEC")
	_, _ = db.Exec("THE WRONG EXEC")

	err := dbmock.ExpectationsWereMet()
	if err == nil {
		t.Fatal("was expecting an error, but there wasn't any")
	}
}

// False Positive - passes despite mismatched Exec
// see #37 issue
func TestRunQueriesWithOrderedShouldNotMeetAllExpectations(t *testing.T) {
	db, dbmock, _ := New()
	dbmock.ExpectQuery("THE FIRST QUERY")
	dbmock.ExpectQuery("THE SECOND QUERY")

	_, _ = db.Query("THE FIRST QUERY")
	_, _ = db.Query("THE WRONG QUERY")

	err := dbmock.ExpectationsWereMet()
	if err == nil {
		t.Fatal("was expecting an error, but there wasn't any")
	}
}

func TestRunExecsWithExpectedErrorMeetsExpectations(t *testing.T) {
	db, dbmock, _ := New()
	dbmock.ExpectExec("THE FIRST EXEC").WillReturnError(fmt.Errorf("big bad bug"))
	dbmock.ExpectExec("THE SECOND EXEC").WillReturnResult(NewResult(0, 0))

	_, _ = db.Exec("THE FIRST EXEC")
	_, _ = db.Exec("THE SECOND EXEC")

	err := dbmock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("all expectations should be met: %s", err)
	}
}

func TestRunQueryWithExpectedErrorMeetsExpectations(t *testing.T) {
	db, dbmock, _ := New()
	dbmock.ExpectQuery("THE FIRST QUERY").WillReturnError(fmt.Errorf("big bad bug"))
	dbmock.ExpectQuery("THE SECOND QUERY").WillReturnRows(NewRows([]string{"col"}).AddRow(1))

	_, _ = db.Query("THE FIRST QUERY")
	_, _ = db.Query("THE SECOND QUERY")

	err := dbmock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("all expectations should be met: %s", err)
	}
}

func TestEmptyRowSet(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rs := NewRows([]string{"id", "title"})

	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WithArgs(5).
		WillReturnRows(rs)

	rows, err := db.Query("SELECT (.+) FROM articles WHERE id = ?", 5)
	if err != nil {
		t.Errorf("error '%s' was not expected while retrieving mock rows", err)
	}

	defer func() {
		if er := rows.Close(); er != nil {
			t.Error("unexpected error while trying to close rows")
		}
	}()

	if rows.Next() {
		t.Error("expected no rows but got one")
	}

	err = mock.ExpectationsWereMet()
	if err != nil {
		t.Fatalf("all expectations should be met: %s", err)
	}
}

// Based on issue #50
func TestPrepareExpectationNotFulfilled(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectPrepare("^BADSELECT$")

	if _, err := db.Prepare("SELECT"); err == nil {
		t.Fatal("prepare should not match expected query string")
	}

	if err := mock.ExpectationsWereMet(); err == nil {
		t.Errorf("was expecting an error, since prepared statement query does not match, but there was none")
	}
}

func TestRollbackThrow(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	// columns to be used for result
	columns := []string{"id", "status"}
	// expect transaction begin
	mock.ExpectBegin()
	// expect query to fetch order, match it with regexp
	mock.ExpectQuery("SELECT (.+) FROM orders (.+) FOR UPDATE").
		WithArgs(1).
		WillReturnRows(NewRows(columns).AddRow(1, 1))
	// expect transaction rollback, since order status is "cancelled"
	mock.ExpectRollback().WillReturnError(fmt.Errorf("rollback failed"))

	// run the cancel order function
	someOrderID := 1
	// call a function which executes expected database operations
	err = cancelOrder(db, someOrderID)
	if err == nil {
		t.Error("an error was expected when rolling back transaction, but got none")
	}

	// ensure all expectations have been met
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectation error: %s", err)
	}
	// Output:
}

func TestUnexpectedBegin(t *testing.T) {
	// Open new mock database
	db, _, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	if _, err := db.Begin(); err == nil {
		t.Error("an error was expected when calling begin, but got none")
	}
}

func TestUnexpectedExec(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin()
	db.Begin()
	if _, err := db.Exec("SELECT 1"); err == nil {
		t.Error("an error was expected when calling exec, but got none")
	}
}

func TestUnexpectedCommit(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin()
	tx, _ := db.Begin()
	if err := tx.Commit(); err == nil {
		t.Error("an error was expected when calling commit, but got none")
	}
}

func TestUnexpectedCommitOrder(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin()
	mock.ExpectRollback().WillReturnError(fmt.Errorf("Rollback failed"))
	tx, _ := db.Begin()
	if err := tx.Commit(); err == nil {
		t.Error("an error was expected when calling commit, but got none")
	}
}

func TestExpectedCommitOrder(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectCommit().WillReturnError(fmt.Errorf("Commit failed"))
	if _, err := db.Begin(); err == nil {
		t.Error("an error was expected when calling begin, but got none")
	}
}

func TestUnexpectedRollback(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin()
	tx, _ := db.Begin()
	if err := tx.Rollback(); err == nil {
		t.Error("an error was expected when calling rollback, but got none")
	}
}

func TestUnexpectedRollbackOrder(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin()

	tx, _ := db.Begin()
	if err := tx.Rollback(); err == nil {
		t.Error("an error was expected when calling rollback, but got none")
	}
}

func TestPrepareExec(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	defer db.Close()
	mock.ExpectBegin()
	ep := mock.ExpectPrepare("INSERT INTO ORDERS\\(ID, STATUS\\) VALUES \\(\\?, \\?\\)")
	for i := 0; i < 3; i++ {
		ep.ExpectExec().WillReturnResult(NewResult(1, 1))
	}
	mock.ExpectCommit()
	tx, _ := db.Begin()
	stmt, err := tx.Prepare("INSERT INTO ORDERS(ID, STATUS) VALUES (?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	for i := 0; i < 3; i++ {
		_, err := stmt.Exec(i, "Hello"+strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestPrepareQuery(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	defer db.Close()
	mock.ExpectBegin()
	ep := mock.ExpectPrepare("SELECT ID, STATUS FROM ORDERS WHERE ID = \\?")
	ep.ExpectQuery().WithArgs(101).WillReturnRows(NewRows([]string{"ID", "STATUS"}).AddRow(101, "Hello"))
	mock.ExpectCommit()
	tx, _ := db.Begin()
	stmt, err := tx.Prepare("SELECT ID, STATUS FROM ORDERS WHERE ID = ?")
	if err != nil {
		t.Fatal(err)
	}
	defer stmt.Close()
	rows, err := stmt.Query(101)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			id     int
			status string
		)
		if rows.Scan(&id, &status); id != 101 || status != "Hello" {
			t.Fatal("wrong query results")
		}

	}
	tx.Commit()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestExpectedCloseError(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectClose().WillReturnError(fmt.Errorf("Close failed"))
	if err := db.Close(); err == nil {
		t.Error("an error was expected when calling close, but got none")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestExpectedCloseOrder(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	defer db.Close()
	mock.ExpectClose().WillReturnError(fmt.Errorf("Close failed"))
	db.Begin()
	if err := mock.ExpectationsWereMet(); err == nil {
		t.Error("expected error on ExpectationsWereMet")
	}
}

func TestExpectedBeginOrder(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	mock.ExpectBegin().WillReturnError(fmt.Errorf("Begin failed"))
	if err := db.Close(); err == nil {
		t.Error("an error was expected when calling close, but got none")
	}
}

func TestPreparedStatementCloseExpectation(t *testing.T) {
	// Open new mock database
	db, mock, err := New()
	if err != nil {
		fmt.Println("error creating mock database")
		return
	}
	defer db.Close()

	ep := mock.ExpectPrepare("INSERT INTO ORDERS").WillBeClosed()
	ep.ExpectExec().WillReturnResult(NewResult(1, 1))

	stmt, err := db.Prepare("INSERT INTO ORDERS(ID, STATUS) VALUES (?, ?)")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := stmt.Exec(1, "Hello"); err != nil {
		t.Fatal(err)
	}

	if err := stmt.Close(); err != nil {
		t.Fatal(err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestExecExpectationErrorDelay(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// test that return of error is delayed
	var delay time.Duration
	delay = 100 * time.Millisecond
	mock.ExpectExec("^INSERT INTO articles").
		WillReturnError(errors.New("slow fail")).
		WillDelayFor(delay)

	start := time.Now()
	res, err := db.Exec("INSERT INTO articles (title) VALUES (?)", "hello")
	stop := time.Now()

	if res != nil {
		t.Errorf("result was not expected, was expecting nil")
	}

	if err == nil {
		t.Errorf("error was expected, was not expecting nil")
	}

	if err.Error() != "slow fail" {
		t.Errorf("error '%s' was not expected, was expecting '%s'", err.Error(), "slow fail")
	}

	elapsed := stop.Sub(start)
	if elapsed < delay {
		t.Errorf("expecting a delay of %v before error, actual delay was %v", delay, elapsed)
	}

	// also test that return of error is not delayed
	mock.ExpectExec("^INSERT INTO articles").WillReturnError(errors.New("fast fail"))

	start = time.Now()
	db.Exec("INSERT INTO articles (title) VALUES (?)", "hello")
	stop = time.Now()

	elapsed = stop.Sub(start)
	if elapsed > delay {
		t.Errorf("expecting a delay of less than %v before error, actual delay was %v", delay, elapsed)
	}
}

func TestOptionsFail(t *testing.T) {
	t.Parallel()
	expected := errors.New("failing option")
	option := func(*sqlmock) error {
		return expected
	}
	db, _, err := New(option)
	defer db.Close()
	if err == nil {
		t.Errorf("missing expecting error '%s' when opening a stub database connection", expected)
	}
}

func TestNewRows(t *testing.T) {
	t.Parallel()
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	columns := []string{"col1", "col2"}

	r := mock.NewRows(columns)
	if len(r.cols) != len(columns) || r.cols[0] != columns[0] || r.cols[1] != columns[1] {
		t.Errorf("expecting to create a row with columns %v, actual colmns are %v", r.cols, columns)
	}
}

// This is actually a test of ExpectationsWereMet. Without a lock around e.fulfilled() inside
// ExpectationWereMet, the race detector complains if e.triggered is being read while it is also
// being written by the query running in another goroutine.
func TestQueryWithTimeout(t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rs := NewRows([]string{"id", "title"}).FromCSVString("5,hello world")

	mock.ExpectQuery("SELECT (.+) FROM articles WHERE id = ?").
		WillDelayFor(15 * time.Millisecond). // Query will take longer than timeout
		WithArgs(5).
		WillReturnRows(rs)

	_, err = queryWithTimeout(10*time.Millisecond, db, "SELECT (.+) FROM articles WHERE id = ?", 5)
	if err == nil {
		t.Errorf("expecting query to time out")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func queryWithTimeout(t time.Duration, db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	rowsChan := make(chan *sql.Rows, 1)
	errChan := make(chan error, 1)

	go func() {
		rows, err := db.Query(query, args...)
		if err != nil {
			errChan <- err
			return
		}
		rowsChan <- rows
	}()

	select {
	case rows := <-rowsChan:
		return rows, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(t):
		return nil, fmt.Errorf("query timed out after %v", t)
	}
}

func Test_sqlmock_Prepare_and_Exec(t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	query := "SELECT name, email FROM users WHERE name = ?"

	mock.ExpectPrepare("SELECT (.+) FROM users WHERE (.+)")
	expected := NewResult(1, 1)
	mock.ExpectExec("SELECT (.+) FROM users WHERE (.+)").
		WillReturnResult(expected)
	expectedRows := mock.NewRows([]string{"id", "name", "email"}).AddRow(1, "test", "test@example.com")
	mock.ExpectQuery("SELECT (.+) FROM users WHERE (.+)").WillReturnRows(expectedRows)

	got, err := mock.(*sqlmock).Prepare(query)
	if err != nil {
		t.Error(err)
		return
	}
	if got == nil {
		t.Error("Prepare () stmt must not be nil")
		return
	}
	result, err := got.Exec([]driver.Value{"test"})
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Results are not equal. Expected: %v, Actual: %v", expected, result)
		return
	}
	rows, err := got.Query([]driver.Value{"test"})
	if err != nil {
		t.Error(err)
		return
	}
	defer rows.Close()
}

type failArgument struct{}

func (f failArgument) Match(_ driver.Value) bool {
	return false
}

func Test_sqlmock_Exec(t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	_, err = mock.(*sqlmock).Exec("", []driver.Value{})
	if err == nil {
		t.Errorf("error expected")
		return
	}

	expected := NewResult(1, 1)
	mock.ExpectExec("SELECT (.+) FROM users WHERE (.+)").
		WillReturnResult(expected).
		WithArgs("test")

	matchErr := errors.New("matcher sqlmock.failArgument could not match 0 argument driver.NamedValue - {Name: Ordinal:1 Value:{}}")
	mock.ExpectExec("SELECT (.+) FROM animals WHERE (.+)").
		WillReturnError(matchErr).
		WithArgs(failArgument{})

	mock.ExpectExec("").WithArgs(failArgument{})

	mock.(*sqlmock).expected = mock.(*sqlmock).expected[1:]
	query := "SELECT name, email FROM users WHERE name = ?"
	result, err := mock.(*sqlmock).Exec(query, []driver.Value{"test"})
	if err != nil {
		t.Error(err)
		return
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Results are not equal. Expected: %v, Actual: %v", expected, result)
		return
	}

	failQuery := "SELECT name, sex FROM animals WHERE sex = ?"
	_, err = mock.(*sqlmock).Exec(failQuery, []driver.Value{failArgument{}})
	if err == nil {
		t.Errorf("error expected")
		return
	}
	mock.(*sqlmock).ordered = false
	_, err = mock.(*sqlmock).Exec("", []driver.Value{failArgument{}})
	if err == nil {
		t.Errorf("error expected")
		return
	}
}

func Test_sqlmock_Query(t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()
	expectedRows := mock.NewRows([]string{"id", "name", "email"}).AddRow(1, "test", "test@example.com")
	mock.ExpectQuery("SELECT (.+) FROM users WHERE (.+)").WillReturnRows(expectedRows)
	query := "SELECT name, email FROM users WHERE name = ?"
	rows, err := mock.(*sqlmock).Query(query, []driver.Value{"test"})
	if err != nil {
		t.Error(err)
		return
	}
	defer rows.Close()
	_, err = mock.(*sqlmock).Query(query, []driver.Value{failArgument{}})
	if err == nil {
		t.Errorf("error expected")
		return
	}
}

func Test_sqlmock_UnexpectedExecWithoutCheckingReturnError (t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	var testing_T testing.T
	mock.FailAndReturnError(&testing_T)

	_, err = db.Exec("UPDATE products SET value = 1 WHERE id = 2")
	if err != nil {
		fmt.Printf("Error: Let's print error and cary on, err %s", err)
	}

	if false == testing_T.Failed() {
		t.Error("test failure expected")
	}
}

func Test_sqlmock_UnexpectedBeginWithoutCheckingReturnError (t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	var testing_T testing.T
	mock.FailAndReturnError(&testing_T)

	_, err = db.Begin()
	if err != nil {
		fmt.Printf("Error: Let's print error and cary on, err %s", err)
	}

	if false == testing_T.Failed() {
		t.Error("test failure expected")
	}
}

func Test_sqlmock_UnexpectedCommitWithoutCheckingReturnError (t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	var testing_T testing.T
	mock.FailAndReturnError(&testing_T)

	mock.ExpectBegin()

	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("Error: Let's print error and cary on, err %s\n", err)
	}

	if testing_T.Failed() {
		t.Error("test failure is not expected")
	}

	tx.Commit()

	if false == testing_T.Failed() {
		t.Error("test failure expected")
	}
}


func Test_sqlmock_UnexpectedQueryWithoutCheckingReturnError (t *testing.T) {
	db, mock, err := New()
	if err != nil {
		t.Errorf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	var testing_T testing.T
	mock.FailAndReturnError(&testing_T)

	_, err = db.Query("SELECT name FROM products WHERE id = 1")
	if err != nil {
		fmt.Printf("Error: Let's print error and cary on, err %s", err)
	}

	if !testing_T.Failed() {
		t.Error("test failure expected")
	}
}

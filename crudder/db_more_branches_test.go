package crudder

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
)

func TestListTablesHandler_DatabaseNotFoundInContext(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tables", nil)

	app.listTablesHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "database not found in context") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}

func TestListTablesHandler_InvalidDatabaseTypeInContext(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tables", nil)
	r = r.WithContext(context.WithValue(r.Context(), userDBKey, "not-a-db"))

	app.listTablesHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid database type in context") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}
}

func TestListTablesHandler_QueryError(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT table_name FROM information_schema\.tables`).WillReturnError(errors.New("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tables", nil)
	r = r.WithContext(context.WithValue(r.Context(), userDBKey, db))

	app.listTablesHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "error fetching tables") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestListTablesHandler_ScanError(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	// Force rows.Scan(&tableName) to fail by returning 2 columns but scanning only 1.
	rows := sqlmock.NewRows([]string{"table_name", "extra"}).AddRow("users", "x")
	mock.ExpectQuery(`SELECT table_name FROM information_schema\.tables`).WillReturnRows(rows)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/tables", nil)
	r = r.WithContext(context.WithValue(r.Context(), userDBKey, db))

	app.listTablesHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "error reading result") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTableStructureHandler_MissingTableParam(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure", nil)

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%q", w.Code, w.Body.String())
	}
	var out map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if out[errMessage] != "the parameter 'table' is mandatory" {
		t.Fatalf("unexpected message: %q", out[errMessage])
	}
}

func TestTableStructureHandler_MissingCookie(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body=%q", w.Code, w.Body.String())
	}
}

func TestTableStructureHandler_SessionNotFound(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: "missing"})

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body=%q", w.Code, w.Body.String())
	}
}

func TestTableStructureHandler_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{
		"tok": {DB: db},
	}}

	mock.ExpectQuery(`FROM information_schema\.columns`).WithArgs("users").WillReturnError(errors.New("boom"))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: "tok"})

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTableStructureHandler_ScanError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{
		"tok": {DB: db},
	}}

	// Force rows.Scan(...) to fail by returning 8 columns while the handler scans 7.
	rows := sqlmock.NewRows([]string{
		"COLUMN_NAME", "DATA_TYPE", "IS_NULLABLE", "COLUMN_DEFAULT",
		"IS_PRIMARY_KEY", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME",
		"EXTRA_COL",
	}).AddRow("id", "int", "NO", nil, false, nil, nil, "x")

	mock.ExpectQuery(`FROM information_schema\.columns`).WithArgs("users").WillReturnRows(rows)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: "tok"})

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Error processing result") {
		t.Fatalf("unexpected body: %q", w.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestTableStructureHandler_ForeignKeyBranch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{
		"tok": {DB: db},
	}}

	// One row where referenced table/column are present to hit the foreign key branch.
	rows := sqlmock.NewRows([]string{
		"COLUMN_NAME", "DATA_TYPE", "IS_NULLABLE", "COLUMN_DEFAULT",
		"IS_PRIMARY_KEY", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME",
	}).AddRow("user_id", "int", "NO", nil, false, "roles", "id")

	mock.ExpectQuery(`FROM information_schema\.columns`).WithArgs("users").WillReturnRows(rows)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)
	r.AddCookie(&http.Cookie{Name: "session_token", Value: "tok"})

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%q", w.Code, w.Body.String())
	}

	var out []ColumnInfo
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("json unmarshal: %v; body=%q", err, w.Body.String())
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 column, got %d", len(out))
	}
	if out[0].ForeignKey == nil || *out[0].ForeignKey != "roles.id" {
		t.Fatalf("expected foreign key roles.id, got %+v", out[0].ForeignKey)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestCrudHandler_MethodNotAllowed(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPatch, "/api/v1/crud/users", nil)
	r = mux.SetURLVars(r, map[string]string{"table": "users"})

	app.crudHandler(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestProcessRowWithColumns_ByteSliceConvertsToString(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"name"}).AddRow([]byte("alice"))
	mock.ExpectQuery(`SELECT`).WillReturnRows(rows)

	r, err := db.Query("SELECT")
	if err != nil {
		t.Fatalf("db.Query: %v", err)
	}
	defer r.Close()

	if !r.Next() {
		t.Fatalf("expected Next() to be true")
	}
	cols, err := r.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}

	item, err := processRowWithColumns(r, cols)
	if err != nil {
		t.Fatalf("processRowWithColumns: %v", err)
	}
	if got, ok := item["name"].(string); !ok || got != "alice" {
		t.Fatalf("expected name=alice (string), got %#v (%T)", item["name"], item["name"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestGetPrimaryKey_NoSession_ReturnsError(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/crud/users/1", nil)
	_, err := app.getPrimaryKey(r, "users")
	if err == nil {
		t.Fatalf("expected error when db is unavailable")
	}
}

// ./crudder/db_test.go
package crudder

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Custom Rows implementations to simulate errors

type errorRows struct {
	sqlmock.Rows
	err error
}

func (r *errorRows) Err() error {
	return r.err
}

type columnsErrorRows struct {
	sqlmock.Rows
}

func (r *columnsErrorRows) Columns() ([]string, error) {
	return nil, fmt.Errorf("mock columns error")
}

type errorValue struct{}

func (e *errorValue) Scan(src interface{}) error {
	return fmt.Errorf("mock scan error")
}

func TestListTablesHandler(t *testing.T) {
	// Configura o mock do banco de dados
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	// Configura o mock para simular uma consulta com sucesso
	mock.ExpectQuery("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()").
		WillReturnRows(sqlmock.NewRows([]string{"table_name"}).AddRow("users").AddRow("orders"))

	// Configura o App
	app := &App{}
	req := httptest.NewRequest("GET", "/tables", nil)

	// Adiciona o banco de dados ao contexto
	ctx := context.WithValue(req.Context(), userDBKey, db)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Chama o handler
	app.listTablesHandler(w, req)

	// Verifica o status da resposta
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Esperado status 200, obtido %d", w.Result().StatusCode)
	}

	// Parseia os valores esperados e obtidos como JSON
	expected := []string{"users", "orders"}
	var obtained []string
	if err := json.Unmarshal(w.Body.Bytes(), &obtained); err != nil {
		t.Fatalf("Erro ao parsear JSON da resposta: %v", err)
	}

	// Compara as estruturas
	if !equalSlices(expected, obtained) {
		t.Errorf("Esperado %v, obtido %v", expected, obtained)
	}

	// Verifica se todas as expectativas do mock foram atendidas
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectativas não atendidas: %v", err)
	}
}

func TestTableStructureHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT c.COLUMN_NAME, c.DATA_TYPE, c.IS_NULLABLE, c.COLUMN_DEFAULT, .* FROM information_schema.columns .*").
		WithArgs("users").
		WillReturnRows(sqlmock.NewRows([]string{
			"COLUMN_NAME", "DATA_TYPE", "IS_NULLABLE", "COLUMN_DEFAULT", "IS_PRIMARY_KEY", "REFERENCED_TABLE_NAME", "REFERENCED_COLUMN_NAME",
		}).AddRow("id", "int", "NO", nil, true, nil, nil).
			AddRow("name", "varchar", "YES", "default_name", false, nil, nil))

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}
	req := httptest.NewRequest("GET", "/table-structure?table=users", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
	w := httptest.NewRecorder()

	app.tableStructureHandler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Esperado status 200, obtido %d", w.Result().StatusCode)
	}
}

func TestUpdateRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
		WithArgs("users").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

	mock.ExpectExec("UPDATE users SET .* WHERE id = ?").
		WithArgs("John", 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}
	req := httptest.NewRequest("PUT", "/crud/users/1", strings.NewReader(`{"name": "John"}`))
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
	w := httptest.NewRecorder()

	app.updateRecord(w, req, "users", 1)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Esperado status 200, obtido %d", w.Result().StatusCode)
	}
}

func TestDeleteRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name           string
		sessionToken   string
		tableName      string
		recordID       int
		mockSetup      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Unauthorized - Session not found",
			sessionToken:   "",
			tableName:      "users",
			recordID:       1,
			mockSetup:      func() {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"message":"Session not found"}`,
		},
		{
			name:         "Error obtaining primary key",
			sessionToken: "mockSession",
			tableName:    "users",
			recordID:     1,
			mockSetup: func() {
				mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
					WithArgs("users").
					WillReturnError(fmt.Errorf("mock error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"Error obtaining primary key: mock error"}`,
		},
		{
			name:         "Error deleting item",
			sessionToken: "mockSession",
			tableName:    "users",
			recordID:     1,
			mockSetup: func() {
				mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				mock.ExpectExec("DELETE FROM users WHERE id = ?").
					WithArgs(1).
					WillReturnError(fmt.Errorf("mock delete error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"Error deleting item: mock delete error"}`,
		},
		{
			name:         "Record not found",
			sessionToken: "mockSession",
			tableName:    "users",
			recordID:     999,
			mockSetup: func() {
				mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				mock.ExpectExec("DELETE FROM users WHERE id = ?").
					WithArgs(999).
					WillReturnResult(sqlmock.NewResult(0, 0))
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"message":"Item not found in database"}`,
		},
		{
			name:         "Successful deletion",
			sessionToken: "mockSession",
			tableName:    "users",
			recordID:     1,
			mockSetup: func() {
				mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				mock.ExpectExec("DELETE FROM users WHERE id = ?").
					WithArgs(1).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"Delete successful","rows_affected":"1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{SessionStore: map[string]*SessionData{}}
			if tt.sessionToken != "" {
				app.SessionStore[tt.sessionToken] = &SessionData{DB: db}
			}

			req := httptest.NewRequest("DELETE", fmt.Sprintf("/crud/%s/%d", tt.tableName, tt.recordID), nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{Name: "session_token", Value: tt.sessionToken})
			}

			w := httptest.NewRecorder()

			tt.mockSetup()

			app.deleteRecord(w, req, tt.tableName, tt.recordID)

			if w.Result().StatusCode != tt.expectedStatus {
				t.Errorf("Esperado status %d, obtido %d", tt.expectedStatus, w.Result().StatusCode)
			}

			if strings.TrimSpace(w.Body.String()) != tt.expectedBody {
				t.Errorf("Esperado body %q, obtido %q", tt.expectedBody, w.Body.String())
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Expectativas não atendidas: %v", err)
			}
		})
	}
}

func TestCreateRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Error creating sqlmock: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}

	t.Run("Success - Record Created", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users .*").
			WithArgs("John", "Doe").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
			WithArgs("users").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Result().StatusCode)
		}
	})

	t.Run("Failure - Invalid Session", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "invalidSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected status 401, got %d", w.Result().StatusCode)
		}
	})

	t.Run("Failure - Invalid JSON Body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John"`)) // Malformed JSON
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Result().StatusCode)
		}
	})

	t.Run("Failure - Database Error on Insert", func(t *testing.T) {
		mock.ExpectExec("INSERT INTO users .*").
			WithArgs("John", "Doe").
			WillReturnError(fmt.Errorf("database error"))

		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", w.Result().StatusCode)
		}
	})

	t.Run("Failure - Invalid Table", func(t *testing.T) {
		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
			WithArgs("invalid_table").WillReturnError(fmt.Errorf("invalid table"))

		req := httptest.NewRequest("POST", "/crud/invalid_table", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "invalid_table")

		if w.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", w.Result().StatusCode)
		}
	})
}

func TestReadRecord(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Error creating sqlmock: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}

	t.Run("Success - Retrieve all records", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
				AddRow(1, "John").
				AddRow(2, "Doe"))

		req := httptest.NewRequest("GET", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.readRecord(w, req, "users")

		assert.Equal(t, http.StatusOK, w.Result().StatusCode, "Expected HTTP status 200 for successful retrieval")
		var response []map[string]interface{}
		err := json.NewDecoder(w.Body).Decode(&response)
		assert.NoError(t, err, "Error decoding response JSON")
		assert.Len(t, response, 2, "Expected 2 records in the response")
	})

	t.Run("Failure - Columns error", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM users").
			WillReturnError(fmt.Errorf("Error querying all records"))

		req := httptest.NewRequest("GET", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.readRecord(w, req, "users")

		assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Expected HTTP status 500 for query error")
		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		assert.NoError(t, err, "Error decoding response JSON")
		assert.Equal(t, "Error querying all records", response[errMessage])
	})

	t.Run("Failure - Error processing rows", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM users").
			WillReturnRows(
				sqlmock.NewRows([]string{"id", "name"}).
					AddRow(1, "John").
					RowError(0, fmt.Errorf("simulated row error")),
			)

		req := httptest.NewRequest("GET", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.readRecord(w, req, "users")

		assert.Equal(t, http.StatusInternalServerError, w.Result().StatusCode, "Expected HTTP status 500 for row error")
		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		assert.NoError(t, err, "Error decoding response JSON")
		assert.Equal(t, "Error processing record", response[errMessage])
	})

	t.Run("Failure - No records found", func(t *testing.T) {
		mock.ExpectQuery("SELECT .* FROM users").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))

		req := httptest.NewRequest("GET", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.readRecord(w, req, "users")

		assert.Equal(t, http.StatusNotFound, w.Result().StatusCode, "Expected HTTP status 404 for no records found")
		var response map[string]string
		err := json.NewDecoder(w.Body).Decode(&response)
		assert.NoError(t, err, "Error decoding response JSON")
		assert.Equal(t, "Item not found in database", response[errMessage])
	})
}

func TestGetPrimaryKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
		WithArgs("users").WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}
	req := httptest.NewRequest("GET", "/crud/users", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

	primaryKey, err := app.getPrimaryKey(req, "users")
	if err != nil {
		t.Fatalf("Erro ao obter chave primária: %v", err)
	}
	if primaryKey != "id" {
		t.Errorf("Esperado 'id', obtido '%s'", primaryKey)
	}
}

func TestGetDBFromSession(t *testing.T) {
	db := &sql.DB{}
	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

	retrievedDB := app.getDBFromSession(req)
	if retrievedDB != db {
		t.Errorf("Esperado %v, obtido %v", db, retrievedDB)
	}
}

func TestProcessRow(t *testing.T) {
	t.Run("Erro ao obter colunas", func(t *testing.T) {
		var rows *sql.Rows = nil
		defer func() {
			if r := recover(); r != nil {
				assert.Contains(t, r, "invalid memory address or nil pointer dereference")
			}
		}()
		_, _ = processRow(rows)
	})

	t.Run("Sucesso ao processar linha", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		mock.ExpectQuery("SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Test"))

		rows, _ := db.Query("SELECT * FROM test")
		defer rows.Close()

		rows.Next() // Move para a primeira linha
		results, err := processRow(rows)
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"id": int64(1), "name": "Test"}, results)
	})
}

func TestProcessRowWithColumns(t *testing.T) {
	t.Run("Erro ao escanear valores", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		mock.ExpectQuery("SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Test"))

		rows, _ := db.Query("SELECT * FROM test")
		defer rows.Close()

		columns := []string{"id", "name", "extra_column"} // Coluna extra
		rows.Next()                                       // Move para a primeira linha
		_, err := processRowWithColumns(rows, columns)
		assert.Error(t, err, "Erro esperado devido à coluna extra")
	})

	t.Run("Sucesso ao processar com colunas", func(t *testing.T) {
		db, mock, _ := sqlmock.New()
		defer db.Close()

		mock.ExpectQuery("SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Test"))

		rows, _ := db.Query("SELECT * FROM test")
		defer rows.Close()

		columns := []string{"id", "name"}
		rows.Next() // Move para a primeira linha
		results, err := processRowWithColumns(rows, columns)
		assert.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"id": int64(1), "name": "Test"}, results)
	})
}

// TestReadRecordByID tests the readRecordByID function with various scenarios.
func TestReadRecordByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Error creating sqlmock: %v", err)
	}
	defer db.Close()
	mock.MatchExpectationsInOrder(true)

	tests := []struct {
		name           string
		sessionToken   string
		sessionExists  bool
		tableName      string
		recordID       int
		mockSetup      func()
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Missing session token",
			sessionToken:   "",
			sessionExists:  false,
			tableName:      "users",
			recordID:       1,
			mockSetup:      func() {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"message":"Session not found"}`,
		},
		{
			name:           "Invalid session token",
			sessionToken:   "invalidSession",
			sessionExists:  false,
			tableName:      "users",
			recordID:       1,
			mockSetup:      func() {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"message":"Session not found"}`,
		},
		{
			name:          "Error querying database",
			sessionToken:  "mockSession",
			sessionExists: true,
			tableName:     "users",
			recordID:      1,
			mockSetup: func() {
				// Mock for primary key
				mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE\(\) AND TABLE_NAME = \? AND CONSTRAINT_NAME = 'PRIMARY'$`).
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				// Mock to simulate error in db.Query
				mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
					WithArgs(1).
					WillReturnError(fmt.Errorf("mock query error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"Error querying the database: mock query error"}`,
		},
		{
			name:          "Error obtaining primary key",
			sessionToken:  "mockSession",
			sessionExists: true,
			tableName:     "users",
			recordID:      1,
			mockSetup: func() {
				// Mock to simulate error in obtaining primary key
				mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE\(\) AND TABLE_NAME = \? AND CONSTRAINT_NAME = 'PRIMARY'$`).
					WithArgs("users").
					WillReturnError(fmt.Errorf("mock primary key error"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"Error obtaining primary key: mock primary key error"}`,
		},
		{
			name:          "Record not found",
			sessionToken:  "mockSession",
			sessionExists: true,
			tableName:     "users",
			recordID:      999,
			mockSetup: func() {
				// Mock for primary key
				mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE\(\) AND TABLE_NAME = \? AND CONSTRAINT_NAME = 'PRIMARY'$`).
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				// Mock for db.Query with no results
				mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
					WithArgs(999).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"message":"Item not found in database"}`,
		},
		{
			name:          "Successful retrieval",
			sessionToken:  "mockSession",
			sessionExists: true,
			tableName:     "users",
			recordID:      1,
			mockSetup: func() {
				// Mock for primary key
				mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE\(\) AND TABLE_NAME = \? AND CONSTRAINT_NAME = 'PRIMARY'$`).
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				// Mock for db.Query returning a valid row
				mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
					WithArgs(1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
						AddRow(1, "testuser"))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":1,"name":"testuser"}`,
		},
		{
			name:          "Successful retrieval with []byte value",
			sessionToken:  "mockSession",
			sessionExists: true,
			tableName:     "users",
			recordID:      1,
			mockSetup: func() {
				// Mock for primary key
				mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE\(\) AND TABLE_NAME = \? AND CONSTRAINT_NAME = 'PRIMARY'$`).
					WithArgs("users").
					WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

				// Mock for db.Query returning a valid row with []byte value
				mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
					WithArgs(1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).
						AddRow(1, []byte("testuser")))
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"id":1,"name":"testuser"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{SessionStore: map[string]*SessionData{}}
			if tt.sessionToken != "" && tt.sessionExists {
				app.SessionStore[tt.sessionToken] = &SessionData{DB: db}
			}

			req := httptest.NewRequest("GET", fmt.Sprintf("/crud/%s/%d", tt.tableName, tt.recordID), nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{Name: "session_token", Value: tt.sessionToken})
			}

			w := httptest.NewRecorder()

			// Set up the mock
			tt.mockSetup()

			// Execute the function
			app.readRecordByID(w, req, tt.tableName, tt.recordID)

			// Logs for debugging
			t.Logf("Response status: %d", w.Result().StatusCode)
			t.Logf("Response body: %s", w.Body.String())

			// Check the returned status
			if w.Result().StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Result().StatusCode)
			}

			// Check the returned body
			if strings.TrimSpace(w.Body.String()) != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			// Confirm that all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unmet expectations: %v", err)
			}
		})
	}
}

func TestCrudHandler(t *testing.T) {
	setup := func(t *testing.T) (*App, *sql.DB, sqlmock.Sqlmock) {
		t.Helper()

		db, mock, err := sqlmock.New()
		require.NoError(t, err)

		app := &App{
			SessionStore: map[string]*SessionData{
				"mockSession": {DB: db},
			},
		}
		return app, db, mock
	}

	primaryKeyRows := func(col string) *sqlmock.Rows {
		return sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow(col)
	}

	pkQuery := regexp.QuoteMeta(`
        SELECT COLUMN_NAME
        FROM information_schema.KEY_COLUMN_USAGE
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
    `)

	t.Run("POST", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		// INSERT (args order depends on map iteration; don't assert WithArgs)
		mock.ExpectExec(`^INSERT INTO users .*`).
			WillReturnResult(sqlmock.NewResult(1, 1))

		// createRecord calls getPrimaryKey after INSERT
		mock.ExpectQuery(pkQuery).
			WithArgs("users").
			WillReturnRows(primaryKeyRows("id"))

		body := bytes.NewBufferString(`{"first_name":"John","last_name":"Doe"}`)
		req := httptest.NewRequest(http.MethodPost, "/crud/users", body)
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"table": "users"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GET", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		rows := sqlmock.NewRows([]string{"id", "name"}).
			AddRow(1, "John").
			AddRow(2, "Jane")

		mock.ExpectQuery(`^SELECT \* FROM users$`).WillReturnRows(rows)

		req := httptest.NewRequest(http.MethodGet, "/crud/users", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var out []map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
		require.Len(t, out, 2)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GET with ID", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		mock.ExpectQuery(pkQuery).
			WithArgs("users").
			WillReturnRows(primaryKeyRows("id"))

		rows := sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John")
		mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
			WithArgs(1).
			WillReturnRows(rows)

		req := httptest.NewRequest(http.MethodGet, "/crud/users/1", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "1"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		var out map[string]interface{}
		require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &out))
		require.Equal(t, float64(1), out["id"])
		require.Equal(t, "John", out["name"])

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("GET with Invalid ID", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		req := httptest.NewRequest(http.MethodGet, "/crud/users/invalid", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "invalid"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("PUT", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		mock.ExpectQuery(pkQuery).
			WithArgs("users").
			WillReturnRows(primaryKeyRows("id"))

		// UPDATE (args order depends on map iteration; don't assert WithArgs)
		mock.ExpectExec(`^UPDATE users SET .* WHERE id = \?$`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		body := bytes.NewBufferString(`{"first_name":"Jane","last_name":"Smith"}`)
		req := httptest.NewRequest(http.MethodPut, "/crud/users/1", body)
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "1"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("PUT with Invalid ID", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		body := bytes.NewBufferString(`{"first_name":"Jane","last_name":"Smith"}`)
		req := httptest.NewRequest(http.MethodPut, "/crud/users/invalid", body)
		req.Header.Set("Content-Type", "application/json")
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "invalid"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DELETE", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		mock.ExpectQuery(pkQuery).
			WithArgs("users").
			WillReturnRows(primaryKeyRows("id"))

		mock.ExpectExec(`^DELETE FROM users WHERE id = \?$`).
			WithArgs(1).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := httptest.NewRequest(http.MethodDelete, "/crud/users/1", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "1"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		expected := `{"message":"Delete successful","rows_affected":"1"}`
		require.JSONEq(t, expected, rr.Body.String())

		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("DELETE with Invalid ID", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		req := httptest.NewRequest(http.MethodDelete, "/crud/users/invalid", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users", "id": "invalid"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Invalid Table Name", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		req := httptest.NewRequest(http.MethodGet, "/crud/invalid$table", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "invalid$table"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Unsupported Method", func(t *testing.T) {
		app, db, mock := setup(t)
		defer db.Close()

		req := httptest.NewRequest(http.MethodPatch, "/crud/users", nil)
		req = mux.SetURLVars(req, map[string]string{"table": "users"})
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})

		rr := httptest.NewRecorder()
		app.crudHandler(rr, req)

		require.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestReadRecord_SessionNotFound(t *testing.T) {
	app := &App{
		SessionStore: make(map[string]*SessionData), // Nenhuma sessão configurada
	}

	// Criar uma requisição com um cookie de sessão inexistente
	req, err := http.NewRequest("GET", "/crud/testtable", nil)
	if err != nil {
		t.Fatalf("Erro ao criar requisição: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "invalidSession"})

	// Capturar a resposta
	recorder := httptest.NewRecorder()

	// Executar a função readRecord
	app.readRecord(recorder, req, "testtable")

	// Verificar o código de status
	if status := recorder.Code; status != http.StatusUnauthorized {
		t.Errorf("Código de status esperado %d, mas obtido %d", http.StatusUnauthorized, status)
	}

	// Verificar a mensagem no corpo da resposta
	expectedBody := `{"message":"Session not found"}`
	if strings.TrimSpace(recorder.Body.String()) != expectedBody {
		t.Errorf("Corpo esperado %q, mas obtido %q", expectedBody, recorder.Body.String())
	}
}

// Função auxiliar para comparar slices
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

func TestGetPrimaryKey_NoSession_ReturnsError(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	r := httptest.NewRequest(http.MethodGet, "/api/v1/crud/users/1", nil)
	_, err := app.getPrimaryKey(r, "users")
	if err == nil {
		t.Fatalf("expected error when db is unavailable")
	}
}

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

func TestTableStructureHandler_MissingCookie(t *testing.T) {
	app := &App{SessionStore: map[string]*SessionData{}}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/table-structure?table=users", nil)

	app.tableStructureHandler(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body=%q", w.Code, w.Body.String())
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

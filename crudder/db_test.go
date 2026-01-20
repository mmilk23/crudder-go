package crudder

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
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
	tests := []struct {
		name           string
		sessionToken   string
		tableName      string
		recordID       int
		mockSetup      func(mock sqlmock.Sqlmock)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Unauthorized - Session not found",
			sessionToken:   "",
			tableName:      "users",
			recordID:       1,
			mockSetup:      func(mock sqlmock.Sqlmock) {},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"message":"Session not found"}`,
		},
		{
			name:         "Database error on delete",
			sessionToken: "mockSession",
			tableName:    "users",
			recordID:     1,
			mockSetup: func(mock sqlmock.Sqlmock) {
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
			mockSetup: func(mock sqlmock.Sqlmock) {
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
			mockSetup: func(mock sqlmock.Sqlmock) {
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
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("Erro ao criar sqlmock: %v", err)
			}
			defer db.Close()
			mock.MatchExpectationsInOrder(false)

			app := &App{SessionStore: map[string]*SessionData{}}
			if tt.sessionToken != "" {
				app.SessionStore[tt.sessionToken] = &SessionData{DB: db}
			}

			tt.mockSetup(mock)

			req := httptest.NewRequest("DELETE", fmt.Sprintf("/crud/%s/%d", tt.tableName, tt.recordID), nil)
			if tt.sessionToken != "" {
				req.AddCookie(&http.Cookie{Name: "session_token", Value: tt.sessionToken})
			}
			w := httptest.NewRecorder()

			app.deleteRecord(w, req, tt.tableName, tt.recordID)

			if w.Result().StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Result().StatusCode)
			}
			if tt.expectedBody != "" {
				var got map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("Could not decode response JSON: %v; body=%q", err, w.Body.String())
				}
				var exp map[string]any
				if err := json.Unmarshal([]byte(tt.expectedBody), &exp); err != nil {
					t.Fatalf("Could not decode expectedBody JSON: %v; expectedBody=%q", err, tt.expectedBody)
				}
				if !reflect.DeepEqual(exp, got) {
					t.Errorf("Expected body %v, got %v", exp, got)
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Expectations not met: %v", err)
			}
		})
	}
}

func TestCreateRecord(t *testing.T) {
	t.Run("Success - Record Created", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("Error creating sqlmock: %v", err)
		}
		defer db.Close()
		mock.MatchExpectationsInOrder(false)

		app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}

		mock.ExpectExec("INSERT INTO users .*").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectQuery("SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE WHERE .*").
			WithArgs("users").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Result().StatusCode)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Expectations not met: %v", err)
		}
	})

	t.Run("Failure - Database Error on Insert", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("Error creating sqlmock: %v", err)
		}
		defer db.Close()
		mock.MatchExpectationsInOrder(false)

		app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}

		mock.ExpectExec("INSERT INTO users .*").
			WillReturnError(fmt.Errorf("database error"))

		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "users")

		if w.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", w.Result().StatusCode)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Expectations not met: %v", err)
		}
	})

	t.Run("Failure - Invalid Table", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("Error creating sqlmock: %v", err)
		}
		defer db.Close()
		mock.MatchExpectationsInOrder(false)

		app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}

		mock.ExpectExec("INSERT INTO invalid_table .*").
			WillReturnError(fmt.Errorf("invalid table"))

		req := httptest.NewRequest("POST", "/crud/invalid_table", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		app.createRecord(w, req, "invalid_table")

		if w.Result().StatusCode != http.StatusInternalServerError {
			t.Errorf("Expected status 500, got %d", w.Result().StatusCode)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("Expectations not met: %v", err)
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
			if tt.expectedBody != "" {
				var got map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatalf("Could not decode response JSON: %v; body=%q", err, w.Body.String())
				}
				var exp map[string]any
				if err := json.Unmarshal([]byte(tt.expectedBody), &exp); err != nil {
					t.Fatalf("Could not decode expectedBody JSON: %v; expectedBody=%q", err, tt.expectedBody)
				}
				if !reflect.DeepEqual(exp, got) {
					t.Errorf("Expected body %v, got %v", exp, got)
				}
			}

			// Confirm that all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unmet expectations: %v", err)
			}
		})
	}
}

func TestCrudHandler(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Error creating sqlmock: %v", err)
	}
	defer db.Close()

	app := &App{SessionStore: map[string]*SessionData{"mockSession": {DB: db}}}
	router := mux.NewRouter()
	router.HandleFunc("/crud/{table}", app.crudHandler).Methods("POST", "GET")
	router.HandleFunc("/crud/{table}/{id}", app.crudHandler).Methods("GET", "PUT", "DELETE")

	// Test POST (Create Record)
	t.Run("POST", func(t *testing.T) {
		mock.ExpectExec(`^INSERT INTO users .*`).
			WithArgs("John", "Doe").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE .*`).
			WithArgs("users").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		req := httptest.NewRequest("POST", "/crud/users", strings.NewReader(`{"first_name": "John", "last_name": "Doe"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("POST expected status 200, got %d", w.Result().StatusCode)
		}
	})

	// Test GET without ID (Read Records)
	t.Run("GET", func(t *testing.T) {
		mock.ExpectQuery(`^SELECT \* FROM users$`).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John").AddRow(2, "Doe"))

		req := httptest.NewRequest("GET", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("GET expected status 200, got %d", w.Result().StatusCode)
		}

		// Optionally, you can check the response body
		var response []map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("Error unmarshalling response body: %v", err)
		}
		if len(response) != 2 {
			t.Errorf("Expected 2 records, got %d", len(response))
		}
	})

	// Test GET with ID (Read Record by ID)
	t.Run("GET with ID", func(t *testing.T) {
		// Mock for primary key retrieval
		mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE .*`).
			WithArgs("users").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		// Mock for fetching the record by ID
		mock.ExpectQuery(`^SELECT \* FROM users WHERE id = \?$`).
			WithArgs(1).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "John"))

		req := httptest.NewRequest("GET", "/crud/users/1", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("GET with ID expected status 200, got %d", w.Result().StatusCode)
		}

		// Optionally, you can check the response body
		var response map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("Error unmarshalling response body: %v", err)
		}
		if response["id"] != float64(1) || response["name"] != "John" {
			t.Errorf("Expected record with id 1 and name 'John', got %+v", response)
		}
	})

	// Test GET with Invalid ID
	t.Run("GET with Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/crud/users/invalid", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("GET with invalid ID expected status 400, got %d", w.Result().StatusCode)
		}

		expectedBody := `{"message":"Invalid ID"}`
		if strings.TrimSpace(w.Body.String()) != expectedBody {
			t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
		}
	})

	// Test PUT (Update Record)
	t.Run("PUT", func(t *testing.T) {
		mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE .*`).
			WithArgs("users").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		mock.ExpectExec(`^UPDATE users SET .* WHERE id = \?$`).
			WithArgs("John", 1).
			WillReturnResult(sqlmock.NewResult(0, 1))

		req := httptest.NewRequest("PUT", "/crud/users/1", strings.NewReader(`{"name": "John"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("PUT expected status 200, got %d", w.Result().StatusCode)
		}
	})

	// Test PUT with Invalid ID
	t.Run("PUT with Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/crud/users/invalid", strings.NewReader(`{"name": "John"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("PUT with invalid ID expected status 400, got %d", w.Result().StatusCode)
		}

		expectedBody := `{"message":"Invalid ID"}`
		if strings.TrimSpace(w.Body.String()) != expectedBody {
			t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
		}
	})

	// Test DELETE (Delete Record)
	t.Run("DELETE", func(t *testing.T) {
		// Mock for primary key retrieval
		mock.ExpectQuery(`^SELECT COLUMN_NAME FROM information_schema\.KEY_COLUMN_USAGE WHERE .*`).
			WithArgs("users").
			WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("id"))

		// Mock for DELETE execution
		mock.ExpectExec(`^DELETE FROM users WHERE id = \?$`).
			WithArgs(1).
			WillReturnResult(sqlmock.NewResult(0, 1)) // 1 row affected

		req := httptest.NewRequest("DELETE", "/crud/users/1", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("DELETE expected status 200, got %d", w.Result().StatusCode)
		}

		expectedBody := `{"message":"Delete successful","rows_affected":"1"}`
		if strings.TrimSpace(w.Body.String()) != expectedBody {
			t.Errorf("DELETE expected body %q, got %q", expectedBody, w.Body.String())
		}
	})

	// Test DELETE with Invalid ID
	t.Run("DELETE with Invalid ID", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/crud/users/invalid", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("DELETE with invalid ID expected status 400, got %d", w.Result().StatusCode)
		}

		expectedBody := `{"message":"Invalid ID"}`
		if strings.TrimSpace(w.Body.String()) != expectedBody {
			t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
		}
	})

	// Test Invalid Table Name
	t.Run("Invalid Table Name", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/crud/invalid$table", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400 for invalid table name, got %d", w.Result().StatusCode)
		}

		expectedBody := `{"message":"Invalid input or table name"}`
		if strings.TrimSpace(w.Body.String()) != expectedBody {
			t.Errorf("Expected body %q, got %q", expectedBody, w.Body.String())
		}
	})

	// Test Unsupported HTTP Method
	t.Run("Unsupported Method", func(t *testing.T) {
		req := httptest.NewRequest("PATCH", "/crud/users", nil)
		req.AddCookie(&http.Cookie{Name: "session_token", Value: "mockSession"})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Result().StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405 for unsupported method, got %d", w.Result().StatusCode)
		}
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

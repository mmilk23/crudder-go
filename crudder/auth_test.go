package crudder

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestLoginHandler(t *testing.T) {
	// Backup and restore the original sqlOpen
	originalSqlOpen := sqlOpen
	defer func() { sqlOpen = originalSqlOpen }()

	tests := []struct {
		name           string
		formData       url.Values
		mockBehavior   func(mock sqlmock.Sqlmock)
		expectedStatus int
		expectedBody   string
		expectSession  bool
	}{
		{
			name:     "Successful login",
			formData: url.Values{"username": {"validuser"}, "password": {"validpass"}, "dbname": {"validdb"}},
			mockBehavior: func(mock sqlmock.Sqlmock) {
				mock.ExpectPing() // Simulate successful ping
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"Login successful"}`,
			expectSession:  true,
		},
		{
			name:     "Invalid credentials",
			formData: url.Values{"username": {"validuser"}, "password": {"validpass"}, "dbname": {"validdb"}},
			mockBehavior: func(mock sqlmock.Sqlmock) {
				mock.ExpectPing().WillReturnError(fmt.Errorf("invalid credentials"))
			},
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"message":"Invalid credentials"}`,
		},
		{
			name:           "Database connection error",
			formData:       url.Values{"username": {"invaliduser"}, "password": {"invalidpass"}, "dbname": {"validdb"}},
			mockBehavior:   func(mock sqlmock.Sqlmock) {}, // No ping expectation for connection error
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   `{"message":"Error connecting to database"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new mock database for each test
			db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			if err != nil {
				t.Fatalf("Error creating sqlmock: %v", err)
			}
			defer db.Close()

			// Override sqlOpen to use the mock database
			sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
				log.Printf("Mock sqlOpen called with: %s", dataSourceName)
				if strings.Contains(dataSourceName, "validuser:validpass") {
					return db, nil
				}
				return nil, fmt.Errorf("mock connection error")
			}

			// Apply mock behavior
			if tt.mockBehavior != nil {
				tt.mockBehavior(mock)
			}

			// Initialize app with an empty session store
			app := &App{SessionStore: make(map[string]*SessionData)}

			// Set environment variable
			os.Setenv("DB_HOST", "localhost")
			defer os.Unsetenv("DB_HOST")

			// Create the HTTP request
			req := httptest.NewRequest("POST", "/login", strings.NewReader(tt.formData.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Create a response recorder
			w := httptest.NewRecorder()

			// Call the login handler
			app.loginHandler(w, req)

			// Validate the response status
			if w.Result().StatusCode != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Result().StatusCode)
			}

			// Validate the response body
			if strings.TrimSpace(w.Body.String()) != tt.expectedBody {
				t.Errorf("Expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			// Validate session creation for successful login
			if tt.expectSession {
				cookies := w.Result().Cookies()
				if len(cookies) == 0 {
					t.Fatal("No session token set")
				}
				sessionToken := cookies[0].Value
				app.Mutex.Lock()
				_, exists := app.SessionStore[sessionToken]
				app.Mutex.Unlock()
				if !exists {
					t.Fatal("Session not found for token")
				}
			}

			// Validate all expectations were met
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("Unmet mock expectations: %v", err)
			}
		})
	}
}

func TestLogoutHandler(t *testing.T) {
	// Configura o mock de banco de dados
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	// Mocka o comportamento de fechamento do banco
	mock.ExpectClose()

	// Inicializa o App com uma sessão válida
	app := &App{SessionStore: make(map[string]*SessionData)}
	sessionToken := "mockSession"
	app.SessionStore[sessionToken] = &SessionData{DB: db}

	// Cria a requisição simulada com o cookie de sessão
	req := httptest.NewRequest("POST", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sessionToken})
	w := httptest.NewRecorder()

	// Executa o handler
	app.logoutHandler(w, req)

	// Verifica se a sessão foi removida
	app.Mutex.Lock()
	if _, exists := app.SessionStore[sessionToken]; exists {
		t.Error("Esperado que a sessão fosse removida")
	}
	app.Mutex.Unlock()

	// Verifica o status da resposta
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Esperado status 200, obtido %d", w.Result().StatusCode)
	}

	// Verifica se todas as expectativas do mock foram atendidas
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("Expectativas não atendidas: %v", err)
	}
}

func TestAuthMiddleware(t *testing.T) {
	// Configura o mock de banco de dados
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Erro ao criar sqlmock: %v", err)
	}
	defer db.Close()

	// Inicializa o App com uma sessão válida
	app := &App{SessionStore: make(map[string]*SessionData)}
	sessionToken := "mockSession"
	app.SessionStore[sessionToken] = &SessionData{DB: db}

	// Rota protegida simulada
	protectedHandler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Teste com cookie válido
	req := httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: sessionToken})
	w := httptest.NewRecorder()

	protectedHandler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("Esperado status 200, obtido %d", w.Result().StatusCode)
	}

	// Teste sem cookie
	req = httptest.NewRequest("GET", "/protected", nil)
	w = httptest.NewRecorder()

	protectedHandler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("Esperado status 401, obtido %d", w.Result().StatusCode)
	}

	// Teste com cookie inválido
	req = httptest.NewRequest("GET", "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: "invalidSession"})
	w = httptest.NewRecorder()

	protectedHandler.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusUnauthorized {
		t.Errorf("Esperado status 401, obtido %d", w.Result().StatusCode)
	}
}

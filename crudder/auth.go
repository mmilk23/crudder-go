package crudder

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type App struct {
	SessionStore map[string]*SessionData // Armazena dados de sessão para cada token de sessão
	Mutex        sync.Mutex              // Protege o acesso a SessionStore em concorrência
}

type contextKey string

const userDBKey contextKey = "userDB"

var sqlOpen = func(driverName, dataSourceName string) (*sql.DB, error) {
	return sql.Open(driverName, dataSourceName)
}

// @Summary Login
// @Description Handler for logging into the database. Creates a session for the user after authenticating with the provided credentials.
// @Tags Authentication
// @Accept application/x-www-form-urlencoded
// @Produce json
// @Param username formData string true "Database username" default(crudder_user)
// @Param password formData string true "Database password" default(crudder_p455w0rd)
// @Param dbname formData string true "Database name" default(crudder_db_test)
// @Success 200 {object} map[string]string "Login successful"
// @Failure 400 {string} string "Username and password are required"
// @Failure 401 {string} string "Invalid credentials"
// @Failure 500 {string} string "Error connecting to the database"
// @Router /login [post]
func (app *App) loginHandler(w http.ResponseWriter, r *http.Request) {
	username := r.FormValue("username")
	password := r.FormValue("password")
	dbName := r.FormValue("dbname")

	//log.Printf("app: %+v, username: %s, password: %s", app, username, password)

	// Input validation
	if username == "" || password == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "Username and password are required")
		return
	}

	// Create DB connection  with the provided credentials
	dbHost := os.Getenv("DB_HOST")
	connStr := fmt.Sprintf("%s:%s@tcp(%s:3306)/%s", username, password, dbHost, dbName)

	db, err := sqlOpen("mysql", connStr)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, errConnDB)
		log.Println("Error opening connection:", err)
		return
	}

	if err := db.Ping(); err != nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errInvalidCred)
		log.Println("Error pinging database:", err)
		return
	}

	// Create a unique session token
	sessionToken := fmt.Sprintf("%d", time.Now().UnixNano())

	// Store the connection in the SessionStore
	app.Mutex.Lock()
	app.SessionStore[sessionToken] = &SessionData{DB: db}
	app.Mutex.Unlock()

	// Set the cookie with the session token
	cookie := &http.Cookie{
		Name:    "session_token",
		Value:   sessionToken,
		Expires: time.Now().Add(5 * time.Minute),
	}
	http.SetCookie(w, cookie)

	writeJSONResponseWithStatus(w, http.StatusOK, map[string]string{errMessage: errLoginOK})
}

// @Summary Logout
// @Description Handler for logging out and closing the database connection associated with the session.
// @Tags Authentication
// @Produce json
// @Success 200 {object} map[string]string "Logout successful"
// @Failure 400 {object} map[string]string "No session found"
// @Router /logout [get]
func (app *App) logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, errNoSessionFound)
		return
	}
	sessionToken := cookie.Value
	app.Mutex.Lock()
	sessionData, exists := app.SessionStore[sessionToken]
	if exists {
		// Fecha a conexão do banco de dados e remove a sessão
		sessionData.DB.Close()
		delete(app.SessionStore, sessionToken)
	}
	app.Mutex.Unlock()

	// Invalida o cookie
	cookie.Expires = time.Now().Add(-time.Hour)
	http.SetCookie(w, cookie)
	writeJSONResponseWithStatus(w, http.StatusOK, map[string]string{errMessage: errLogoutOK})
}

// Middleware to check if user is authenticated and get user connection
func (app *App) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			writeJSONResponseWithStatus(w, http.StatusUnauthorized, map[string]string{errMessage: errUnauthorized})
			return
		}
		sessionToken := cookie.Value
		app.Mutex.Lock()
		sessionData, exists := app.SessionStore[sessionToken]
		app.Mutex.Unlock()
		if !exists {
			writeJSONResponseWithStatus(w, http.StatusUnauthorized, map[string]string{errMessage: errUnauthorized})
			return
		}
		// Store the connection in the context for the next handler
		ctx := context.WithValue(r.Context(), userDBKey, sessionData.DB)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

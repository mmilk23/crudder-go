package crudder

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

// struct to store the connection of each user session
type SessionData struct {
	DB *sql.DB
}

// struct represents a colum data
type ColumnInfo struct {
	ColumnName       string  `json:"column_name"`
	DataType         string  `json:"data_type"`
	IsNullable       bool    `json:"is_nullable"`
	ColumnDefault    *string `json:"column_default,omitempty"`
	IsPrimaryKey     bool    `json:"is_primary_key"`
	ForeignKey       *string `json:"foreign_key,omitempty"`
	ReferencedTable  *string `json:"referenced_table,omitempty"`
	ReferencedColumn *string `json:"referenced_column,omitempty"`
}

// @Summary List Tables
// @Description Retrieves the names of all tables in the current database schema. Requires a valid database connection from the context.
// @Tags Database
// @Produce json
// @Success 200 {array} string "List of table names"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /tables [get]
func (app *App) listTablesHandler(w http.ResponseWriter, r *http.Request) {
	// Recupera a conexão do banco de dados do contexto
	userDB := r.Context().Value(userDBKey)
	if userDB == nil {
		http.Error(w, "database not found in context", http.StatusInternalServerError)
		return
	}

	db, ok := userDB.(*sql.DB)
	if !ok {
		http.Error(w, "invalid database type in context", http.StatusInternalServerError)
		return
	}

	// Executa a consulta para listar as tabelas
	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE()")
	if err != nil {
		http.Error(w, "error fetching tables", http.StatusInternalServerError)
		log.Println("error fetching tables:", err)
		return
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			http.Error(w, "error reading result", http.StatusInternalServerError)
			log.Println("error reading result:", err)
			return
		}
		tables = append(tables, tableName)
	}
	w.Header().Set(headerContentType, headerContentTypeJSON)
	json.NewEncoder(w).Encode(tables)
}

// @Summary Get Table Structure
// @Description Handler for retrieving the structure of a specific table, including primary and foreign keys.
// @Tags Database
// @Produce json
// @Param table query string true "Table name" default(users)
// @Success 200 {array} ColumnInfo "Table structure retrieved successfully"
// @Failure 400 {object} map[string]string "The parameter 'table' is mandatory"
// @Failure 401 {object} map[string]string "Unauthorized"
// @Failure 500 {object} map[string]string "Error querying table structure or processing result"
// @Router /table-structure [get]
func (app *App) tableStructureHandler(w http.ResponseWriter, r *http.Request) {
	tableName := r.URL.Query().Get("table")
	if tableName == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "the parameter 'table' is mandatory")
		return
	}

	// retreive db connection
	cookie, err := r.Cookie("session_token")
	if err != nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errUnauthorized)
		return
	}
	app.Mutex.Lock()
	sessionData, exists := app.SessionStore[cookie.Value]
	app.Mutex.Unlock()
	if !exists {
		WriteErrorResponse(w, http.StatusUnauthorized, errUnauthorized)
		return
	}

	// Query the table structure with primary and foreign keys
	query := `
        SELECT 
            c.COLUMN_NAME, 
            c.DATA_TYPE, 
            c.IS_NULLABLE, 
            c.COLUMN_DEFAULT,
            IF(k.CONSTRAINT_NAME = 'PRIMARY', TRUE, FALSE) AS IS_PRIMARY_KEY,
            k.REFERENCED_TABLE_NAME,
            k.REFERENCED_COLUMN_NAME
        FROM information_schema.columns AS c
        LEFT JOIN information_schema.key_column_usage AS k
        ON c.TABLE_NAME = k.TABLE_NAME 
           AND c.COLUMN_NAME = k.COLUMN_NAME 
           AND k.TABLE_SCHEMA = DATABASE()
        WHERE c.table_schema = DATABASE() AND c.table_name = ?
    `
	rows, err := sessionData.DB.Query(query, tableName)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "Error querying table structure")
		log.Println("Erro ao consultar estrutura:", err)
		return
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var columnDefault sql.NullString
		var isNullableStr string
		var referencedTable sql.NullString
		var referencedColumn sql.NullString
		var isPrimaryKey bool

		if err := rows.Scan(&col.ColumnName, &col.DataType, &isNullableStr, &columnDefault, &isPrimaryKey, &referencedTable, &referencedColumn); err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "Error processing result")
			log.Println("Error processing result:", err)
			return
		}
		// convert isNullableStr ("YES"/"NO") para bool
		col.IsNullable = isNullableStr == "YES"

		// Assigns the values ​​of primary and foreign keys
		col.IsPrimaryKey = isPrimaryKey

		// converts sql.NullString to *string para ColumnDefault
		if columnDefault.Valid {
			col.ColumnDefault = new(string)
			*col.ColumnDefault = columnDefault.String
		} else {
			col.ColumnDefault = nil
		}

		// converts sql.NullString to *string para ForeignKey
		if !referencedTable.Valid || !referencedColumn.Valid {
			col.ReferencedTable = nil
			col.ReferencedColumn = nil
			col.ForeignKey = nil
		} else {
			col.ReferencedTable = &referencedTable.String
			col.ReferencedColumn = &referencedColumn.String
			foreignKey := fmt.Sprintf("%s.%s", referencedTable.String, referencedColumn.String)
			col.ForeignKey = &foreignKey
		}

		columns = append(columns, col)
	}

	// Retorna a estrutura da tabela em formato JSON
	w.Header().Set(headerContentType, headerContentTypeJSON)
	json.NewEncoder(w).Encode(columns)
}

// @Summary Update Record
// @Description Updates a record in the specified table based on the provided ID. This endpoint requires a valid session token.
// @Tags CRUD
// @Accept json
// @Produce json
// @Param table path string true "Name of the table" default(users)
// @Param id path int true "ID of the record to update" default(3)
// @Param body body object true "JSON object with updated fields" example({"username": "user3changed","pwd": "456456"})
// @Success 200 {object} map[string]interface{} "Record updated successfully"
// @Failure 400 {string} string "Invalid input or JSON decoding error"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "Record not found"
// @Failure 500 {string} string "Internal server error"
// @Router /crud/{table}/{id} [put]
func (app *App) updateRecord(w http.ResponseWriter, r *http.Request, tableName string, id int) {
	var item map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "Invalid input or JSON decoding error")
		return
	}

	db := app.getDBFromSession(r)
	if db == nil {
		return
	}

	// Obter a coluna de chave primária
	primaryKey, err := app.getPrimaryKey(r, tableName)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf(errFindPrimaryKey, err))
		return
	}

	columns := make([]string, 0, len(item))
	values := make([]interface{}, 0, len(item))

	for col, val := range item {
		columns = append(columns, fmt.Sprintf("%s = ?", col))
		values = append(values, val)
	}

	values = append(values, id)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", tableName, strings.Join(columns, ", "), primaryKey)

	result, err := db.Exec(query, values...)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error updating Item: %v", err))
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		WriteErrorResponse(w, http.StatusNotFound, "Record not found or not updated")
		return
	}

	item[primaryKey] = id
	w.Header().Set(headerContentType, headerContentTypeJSON)
	json.NewEncoder(w).Encode(item)
}

// @Summary Delete Record
// @Description Deletes a record in the specified table based on the provided ID. This endpoint requires a valid session token.
// @Tags CRUD
// @Param table path string true "Name of the table" default(users)
// @Param id path int true "ID of the record to delete" default(3)
// @Success 200 {object} map[string]string "Delete successful with affected rows"
// @Failure 400 {object} map[string]string "Invalid table name or ID"
// @Failure 401 {object} map[string]string "Unauthorized - Session not found"
// @Failure 404 {object} map[string]string "Record not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /crud/{table}/{id} [delete]
func (app *App) deleteRecord(w http.ResponseWriter, r *http.Request, tableName string, id int) {
	db := app.getDBFromSession(r)
	if db == nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errSessionNotFound)
		return
	}

	// Obter a coluna de chave primária
	primaryKey, err := app.getPrimaryKey(r, tableName)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", tableName, primaryKey)
	result, err := db.Exec(query, id)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting item: %v", err))
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected == 0 {
		WriteErrorResponse(w, http.StatusNotFound, errItemNotFound)
		return
	}

	// Retorna mensagem de sucesso com o total de linhas afetadas
	response := map[string]string{
		"message":       "Delete successful",
		"rows_affected": fmt.Sprintf("%d", rowsAffected),
	}

	writeJSONResponseWithStatus(w, http.StatusOK, response)
}

// @Summary Create Record
// @Description Creates a new record in the specified table. This endpoint requires a valid session token.
// @Tags CRUD
// @Accept json
// @Produce json
// @Param table path string true "Name of the table" default(users)
// @Param body body object true "JSON object for the new record" example({"username": "newuser","pwd": "789789"})
// @Success 201 {object} map[string]interface{} "Record created successfully"
// @Failure 400 {string} string "Invalid input or JSON decoding error"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /crud/{table} [post]
func (app *App) createRecord(w http.ResponseWriter, r *http.Request, tableName string) {
	var item map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "Invalid input or JSON decoding error")
		return
	}

	db := app.getDBFromSession(r)
	if db == nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errSessionNotFound)
		return
	}

	columns := make([]string, 0, len(item))
	values := make([]interface{}, 0, len(item))
	placeholders := make([]string, 0, len(item))

	for col, val := range item {
		columns = append(columns, col)
		values = append(values, val)
		placeholders = append(placeholders, "?")
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableName, strings.Join(columns, ","), strings.Join(placeholders, ","))
	result, err := db.Exec(query, values...)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, "Error inserting Item")
		return
	}

	id, _ := result.LastInsertId()
	primaryKey, err := app.getPrimaryKey(r, tableName)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf(errFindPrimaryKey, err))
		return
	}

	item[primaryKey] = id

	writeJSONResponseWithStatus(w, http.StatusOK, item)

}

// read all records
func (app *App) readRecord(w http.ResponseWriter, r *http.Request, tableName string) {
	db := app.getDBFromSession(r)
	if db == nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errSessionNotFound)
		return
	}

	// read all records
	app.readAllRecords(w, db, tableName)
}

// @Summary Retrieve a Record by ID
// @Description Fetches a specific record from the specified table using its primary key. Requires a valid session token.
// @Tags CRUD
// @Produce json
// @Param table path string true "Name of the table" default(users)
// @Param id path int true "ID of the record to retrieve" default(1)
// @Success 200 {object} map[string]interface{} "The requested record"
// @Failure 400 {object} map[string]string "Invalid ID or request parameters"
// @Failure 401 {object} map[string]string "Unauthorized or session not found"
// @Failure 404 {object} map[string]string "Record not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /crud/{table}/{id} [get]
func (app *App) readRecordByID(w http.ResponseWriter, r *http.Request, tableName string, id int) {
	db := app.getDBFromSession(r)
	if db == nil {
		WriteErrorResponse(w, http.StatusUnauthorized, errSessionNotFound)
		return
	}

	primaryKey, err := app.getPrimaryKey(r, tableName)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := fmt.Sprintf("SELECT * FROM %s WHERE %s = ?", tableName, primaryKey)
	rows, err := db.Query(query, id)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("Error querying the database: %s", err.Error()))
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, errColumnNotFound)
		return
	}

	if rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, errScanRow)
			return
		}

		item := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			switch v := val.(type) {
			case []byte:
				item[col] = string(v)
			default:
				item[col] = v
			}
		}

		writeJSONResponseWithStatus(w, http.StatusOK, item)
		return
	}

	if err := rows.Err(); err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, errRows)
		return
	}
	WriteErrorResponse(w, http.StatusNotFound, errItemNotFound)
}

// @Summary Retrieve All Records
// @Description Retrieves all records from the specified table. This endpoint requires a valid session token.
// @Tags CRUD
// @Produce json
// @Param tableName path string true "Name of the table to query" default(users)
// @Success 200 {array} map[string]interface{} "List of all records"
// @Failure 401 {object} map[string]string "Unauthorized or session not found"
// @Failure 500 {object} map[string]string "Internal server error"
// @Router /crud/{tableName} [get]
func (app *App) readAllRecords(w http.ResponseWriter, db *sql.DB, tableName string) {
	query := fmt.Sprintf("SELECT * FROM %s", tableName)
	rows, err := db.Query(query)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, errqryAllRecords)
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, errColumnNotFound)
		return
	}

	var items []map[string]interface{}
	for rows.Next() {
		item, err := processRowWithColumns(rows, columns)
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, errRecords) // Handle row processing errors
			return
		}
		items = append(items, item)
	}

	if err = rows.Err(); err != nil { // Ensure no errors occurred during iteration
		WriteErrorResponse(w, http.StatusInternalServerError, errRecords)
		return
	}

	if len(items) == 0 { // Return 404 if no records are found
		WriteErrorResponse(w, http.StatusNotFound, errItemNotFound)
		return
	}

	// Respond with the retrieved records
	writeJSONResponseWithStatus(w, http.StatusOK, items)
}

func processRow(rows *sql.Rows) (map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return processRowWithColumns(rows, columns)
}

func processRowWithColumns(rows *sql.Rows, columns []string) (map[string]interface{}, error) {
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	item := make(map[string]interface{})
	for i, col := range columns {
		val := values[i]
		switch v := val.(type) {
		case []byte:
			item[col] = string(v) // Converte byte arrays para string
		default:
			item[col] = v
		}
	}
	return item, nil
}

// Function to obtain the primary key column of a table
func (app *App) getPrimaryKey(r *http.Request, tableName string) (string, error) {
	db := app.getDBFromSession(r)
	if db == nil {
		return "", fmt.Errorf("conexão ao banco de dados não disponível")
	}

	query := `
        SELECT COLUMN_NAME
        FROM information_schema.KEY_COLUMN_USAGE
        WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
    `

	var primaryKey string
	err := db.QueryRow(query, tableName).Scan(&primaryKey)
	if err != nil {
		return "", fmt.Errorf(errFindPrimaryKey, err)
	}

	return primaryKey, nil
}

// Helper function to get database connection from user session
func (app *App) getDBFromSession(r *http.Request) *sql.DB {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		log.Println("Erro getting cookie:", err)
		return nil
	}

	app.Mutex.Lock()
	sessionData, exists := app.SessionStore[cookie.Value]
	app.Mutex.Unlock()
	if !exists {
		log.Println("Sesssion not found to token:", cookie.Value)
		return nil
	}

	return sessionData.DB
}

// crud endpoint switcher
func (app *App) crudHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	tableName := vars["table"]
	idParam := vars["id"] // Obtém o id da URL, se presente
	if !isAlphaNumeric(tableName) {
		WriteErrorResponse(w, http.StatusBadRequest, errInvalidInput)
		return
	}

	switch r.Method {
	case "POST":
		app.createRecord(w, r, tableName)
	case "GET":
		if idParam != "" {
			// Converte o idParam para int e chama readRecord com id específico
			id, err := strconv.Atoi(idParam)
			if err != nil {
				WriteErrorResponse(w, http.StatusBadRequest, errInvalidID)
				return
			}
			app.readRecordByID(w, r, tableName, id)
		} else {
			app.readRecord(w, r, tableName)
		}
	case "PUT":
		id, err := strconv.Atoi(idParam)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, errInvalidID)
			return
		}
		app.updateRecord(w, r, tableName, id)
	case "DELETE":
		id, err := strconv.Atoi(idParam)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, errInvalidID)
			return
		}
		app.deleteRecord(w, r, tableName, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

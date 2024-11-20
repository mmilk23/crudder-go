package crudder

import (
	"encoding/json"
	"net/http"
)

const (
	// HTTP Headers
	headerContentType     = "Content-Type"
	headerContentTypeJSON = "application/json"
	errSessionNotFound    = "Session not found"

	// Error Messages
	errMessage        = "message"
	errConnDB         = "Error connecting to database"
	errFindPrimaryKey = "Error obtaining primary key: %v"
	errQryDatabase    = "Error querying the database: %v"
	errItemNotFound   = "Item not found in database"
	errqryAllRecords  = "Error querying all records"
	errColumnNotFound = "Error obtaining columns"
	errRecords        = "Error processing record"
	errRows           = "Error processing rows"
	errScanRow        = "Error scanning the row"
	errInvalidID      = "Invalid ID"
	errInvalidInput   = "Invalid input or table name"
	errInvalidCred    = "Invalid credentials"
	errLoginOK        = "Login successful"
	errLogoutOK       = "Logout successful"
	errUnauthorized   = "Unauthorized"
	errNoSessionFound = "No session found"
)

// Function to validate if the table name is alphanumeric
func isAlphaNumeric(str string) bool {
	if len(str) == 0 { // Verifica se a string é vazia
		return false
	}
	for _, c := range str {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// Helper function to write error responses
func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	response := map[string]string{errMessage: message}
	w.Header().Set(headerContentType, headerContentTypeJSON)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(response)
}

func writeJSONResponseWithStatus(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set(headerContentType, headerContentTypeJSON)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

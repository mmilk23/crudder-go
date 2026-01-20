package crudder

import "testing"

func TestSqlOpen_DefaultFunc_IsCallable(t *testing.T) {
	// sql.Open does not establish a network connection; it only validates the driver.
	// This covers the default sqlOpen func value.
	db, err := sqlOpen("mysql", "u:p@tcp(localhost:3306)/d")
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if db == nil {
		t.Fatalf("expected non-nil db")
	}
	_ = db.Close()
}

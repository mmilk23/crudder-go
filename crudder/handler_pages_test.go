package crudder

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createMinimalTemplates creates minimal templates under a temporary working directory
// so renderTemplate() can parse relative paths like "static/html/...".
func createMinimalTemplates(t *testing.T) (restoreCwd func()) {
	t.Helper()

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}

	tmp := t.TempDir()

	// Create dirs
	mustMkdirAll(t, filepath.Join(tmp, "static", "html", "template"))
	mustMkdirAll(t, filepath.Join(tmp, "static", "html"))

	// Base templates define template names equal to their filenames and call a shared "body".
	writeFile(t, filepath.Join(tmp, "static", "html", "template", "launch.html"),
		`{{define "launch.html"}}<html><head><title>{{.Title}}</title></head><body>{{template "body" .}}</body></html>{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "template", "base.html"),
		`{{define "base.html"}}<html><head><title>{{.Title}}</title></head><body>{{template "body" .}}</body></html>{{end}}`)

	// Content templates (each defines "body" for that renderTemplate call)
	writeFile(t, filepath.Join(tmp, "static", "html", "login.html"),
		`{{define "body"}}LOGIN_CONTENT {{.Title}}{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "welcome.html"),
		`{{define "body"}}WELCOME_CONTENT {{.Title}}{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "table_crud.html"),
		`{{define "body"}}TABLE_CRUD_CONTENT {{.Title}}{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "table_crud_edit.html"),
		`{{define "body"}}TABLE_CRUD_EDIT_CONTENT {{.Title}}{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "table_crud_add.html"),
		`{{define "body"}}TABLE_CRUD_ADD_CONTENT {{.Title}}{{end}}`)
	writeFile(t, filepath.Join(tmp, "static", "html", "table_crud_delete.html"),
		`{{define "body"}}TABLE_CRUD_DELETE_CONTENT {{.Title}}{{end}}`)

	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir(%s): %v", tmp, err)
	}

	return func() {
		_ = os.Chdir(orig)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func TestRenderTemplate_Success(t *testing.T) {
	restore := createMinimalTemplates(t)
	defer restore()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/welcome", nil)

	// Any struct is fine; handlers use Title only.
	data := struct{ Title string }{Title: "MyTitle"}

	renderTemplate(w, "base.html", "welcome.html", data)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "WELCOME_CONTENT") {
		t.Fatalf("expected body to contain WELCOME_CONTENT, got %q", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "MyTitle") {
		t.Fatalf("expected body to contain title, got %q", w.Body.String())
	}
	_ = req // keep request in case of future extension
}

func TestRenderTemplate_MissingFiles_Returns500(t *testing.T) {
	restore := createMinimalTemplates(t)
	defer restore()

	w := httptest.NewRecorder()
	data := struct{ Title string }{Title: "X"}

	// Intentionally missing base/content.
	renderTemplate(w, "does_not_exist.html", "nope.html", data)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Error loading template") {
		t.Fatalf("expected error body, got %q", w.Body.String())
	}
}

func TestPageHandlers_Render200(t *testing.T) {
	restore := createMinimalTemplates(t)
	defer restore()

	app := &App{SessionStore: map[string]*SessionData{}}

	cases := []struct {
		name   string
		path   string
		call   func(w http.ResponseWriter, r *http.Request)
		marker string
	}{
		{"loginPageHandler", "/login", app.loginPageHandler, "LOGIN_CONTENT"},
		{"welcomePageHandler", "/welcome", app.welcomePageHandler, "WELCOME_CONTENT"},
		{"tableCrudPageHandler", "/table-crud", app.tableCrudPageHandler, "TABLE_CRUD_CONTENT"},
		{"tableCrudEditPageHandler", "/table-crud-edit", app.tableCrudEditPageHandler, "TABLE_CRUD_EDIT_CONTENT"},
		{"tableCrudAddPageHandler", "/table-crud-add", app.tableCrudAddPageHandler, "TABLE_CRUD_ADD_CONTENT"},
		{"tableCrudDeletePageHandler", "/table-crud-delete", app.tableCrudDeletePageHandler, "TABLE_CRUD_DELETE_CONTENT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)

			tc.call(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d; body=%q", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.marker) {
				t.Fatalf("expected body to contain %q, got %q", tc.marker, w.Body.String())
			}
		})
	}
}

func TestInitTemplates_PanicsWhenNoAbsoluteStaticHTML(t *testing.T) {
	// InitTemplates uses an absolute glob "/static/html/*.html".
	// In most dev environments this path won't exist, so template.Must should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic from InitTemplates() when /static/html/*.html is missing")
		}
	}()
	InitTemplates()
}

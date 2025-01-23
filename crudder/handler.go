package crudder

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	_ "github.com/milklabs/crudder-go/docs"
	httpSwagger "github.com/swaggo/http-swagger"
)

type ServerInterface interface {
	ListenAndServe(addr string, handler http.Handler) error
}

type RealServer struct{}

func (r RealServer) ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

var tmpl *template.Template

func LoadEnv() error {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("alert: .env file not found")
		return err
	}
	return nil
}

func SetupRouter(app *App) *mux.Router {
	router := mux.NewRouter()

	apiRouter := router.PathPrefix("/api/v1").Subrouter()

	apiRouter.HandleFunc("/login", app.loginHandler).Methods("POST")
	apiRouter.HandleFunc("/logout", app.logoutHandler).Methods("GET")
	apiRouter.Handle("/crud/{table}", app.authMiddleware(http.HandlerFunc(app.crudHandler))).Methods("POST", "GET")
	apiRouter.Handle("/crud/{table}/{id:[0-9]+}", app.authMiddleware(http.HandlerFunc(app.crudHandler))).Methods("GET", "PUT", "DELETE")
	apiRouter.Handle("/tables", app.authMiddleware(http.HandlerFunc(app.listTablesHandler)))
	apiRouter.Handle("/table-structure", app.authMiddleware(http.HandlerFunc(app.tableStructureHandler)))

	router.HandleFunc("/login", app.loginPageHandler)
	router.HandleFunc("/welcome", app.welcomePageHandler)
	router.HandleFunc("/table-crud", app.tableCrudPageHandler)
	router.HandleFunc("/table-crud-add", app.tableCrudAddPageHandler)
	router.HandleFunc("/table-crud-edit", app.tableCrudEditPageHandler)
	router.HandleFunc("/table-crud-delete", app.tableCrudDeletePageHandler)

	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)
	return router
}

func (app *App) loginPageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: login",
	}
	renderTemplate(w, "launch.html", "login.html", data)
}

func (app *App) welcomePageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: welcome",
	}
	renderTemplate(w, "base.html", "welcome.html", data)
}

func (app *App) tableCrudPageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: ",
	}
	renderTemplate(w, "base.html", "table_crud.html", data)
}

func (app *App) tableCrudEditPageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: ",
	}
	renderTemplate(w, "base.html", "table_crud_edit.html", data)
}

func (app *App) tableCrudAddPageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: ",
	}
	renderTemplate(w, "base.html", "table_crud_add.html", data)
}

func (app *App) tableCrudDeletePageHandler(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "Crudder Go:: ",
	}
	renderTemplate(w, "base.html", "table_crud_delete.html", data)
}

func InitTemplates() {
	tmpl = template.Must(template.ParseGlob("/static/html/*.html"))
}

func renderTemplate(w http.ResponseWriter, baseTemplate string, contentTemplate string, data interface{}) {
	tmpl, err := template.ParseFiles(
		fmt.Sprintf("static/html/template/%s", baseTemplate),
		fmt.Sprintf("static/html/%s", contentTemplate),
	)
	if err != nil {
		http.Error(w, "Error loading template: "+err.Error(), http.StatusInternalServerError)
		return
	}

	err = tmpl.ExecuteTemplate(w, baseTemplate, data)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
	}
}

func StartServer(server ServerInterface) {
	if err := LoadEnv(); err != nil {
		log.Println(".env file not found")
	}

	app := &App{
		SessionStore: make(map[string]*SessionData),
	}

	router := SetupRouter(app)
	log.Println("Server running at http://localhost:9091")
	err := server.ListenAndServe(":9091", router)
	if err != nil {
		log.Fatal(err)
	}
}

func StartServerDefault() {
	StartServer(RealServer{})
}

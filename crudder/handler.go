package crudder

import (
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"

	_ "github.com/milklabs/crudder-go/docs"
	httpSwagger "github.com/swaggo/http-swagger"
)

// ServerInterface define a interface para iniciar o servidor
type ServerInterface interface {
	ListenAndServe(addr string, handler http.Handler) error
}

// RealServer é a implementação padrão de ServerInterface
type RealServer struct{}

// ListenAndServe inicia o servidor real
func (r RealServer) ListenAndServe(addr string, handler http.Handler) error {
	return http.ListenAndServe(addr, handler)
}

// LoadEnv tenta carregar as variáveis de ambiente do arquivo .env
func LoadEnv() error {
	if err := godotenv.Load(".env"); err != nil {
		log.Println("Aviso: arquivo .env não encontrado")
		return err
	}
	return nil
}

func SetupRouter(app *App) *mux.Router {
	router := mux.NewRouter()
	router.HandleFunc("/login", app.loginHandler).Methods("POST")
	router.HandleFunc("/logout", app.logoutHandler).Methods("GET")

	router.Handle("/crud/{table}", app.authMiddleware(http.HandlerFunc(app.crudHandler))).Methods("POST", "GET")
	router.Handle("/crud/{table}/{id:[0-9]+}", app.authMiddleware(http.HandlerFunc(app.crudHandler))).Methods("GET", "PUT", "DELETE")
	router.Handle("/tables", app.authMiddleware(http.HandlerFunc(app.listTablesHandler)))
	router.Handle("/table-structure", app.authMiddleware(http.HandlerFunc(app.tableStructureHandler)))

	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	return router
}

func StartServer(server ServerInterface) {
	if err := LoadEnv(); err != nil {
		log.Println("Continuando sem .env")
	}

	app := &App{
		SessionStore: make(map[string]*SessionData),
	}

	router := SetupRouter(app)
	log.Println("Server running at http://localhost:8080")
	err := server.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal(err)
	}
}

func StartServerDefault() {
	StartServer(RealServer{})
}

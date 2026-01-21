// main.go
package main

import "github.com/milklabs/crudder-go/crudder"

// VariableStartServerDefault permite mockar StartServerDefault nos testes
var VariableStartServerDefault = crudder.StartServerDefault

// mainFn permite testar o fluxo do main sem depender do runtime executar main()
var mainFn = func() {
	VariableStartServerDefault()
}

func runServer() {
	VariableStartServerDefault()
}

func main() {
	mainFn()
}

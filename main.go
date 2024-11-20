package main

import "github.com/milklabs/crudder-go/crudder"

// VariableStartServerDefault permite mockar StartServerDefault nos testes
var VariableStartServerDefault = crudder.StartServerDefault

func runServer() {
	VariableStartServerDefault()
}

func main() {
	runServer()
}

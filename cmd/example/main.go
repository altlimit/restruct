package main

import (
	"log"
	"net/http"

	"github.com/altlimit/restruct"
)

type MyService struct {
}

func (m *MyService) Products(r *http.Request) interface{} {

	return "products"
}

func (m *MyService) Products_0(r *http.Request, w http.ResponseWriter) interface{} {

	return "Product " + restruct.Param(r, "0")
}

func main() {
	restruct.Handle("/api/v1/", restruct.NewHandler(&MyService{}))
	port := "8090"
	log.Println("Listening " + port)
	http.ListenAndServe(":"+port, nil)
}

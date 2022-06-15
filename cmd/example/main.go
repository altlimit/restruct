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

	log.Println("Product " + restruct.Param(r.Context(), "0"))
	return nil
}

func main() {
	svc := restruct.NewHandler(&MyService{})
	svc.AddService("/{tag}/", &MyService{})
	restruct.Handle("/api/v1/", svc)
	port := "8090"
	log.Println("Listening " + port)
	http.ListenAndServe(":"+port, nil)
}

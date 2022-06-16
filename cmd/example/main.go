package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/altlimit/restruct"
)

type MyService struct {
}

func (m *MyService) Non(r *http.Request) restruct.Response {
	return restruct.Response{
		Status:  http.StatusBadRequest,
		Content: map[string]string{"Hello": "worl"},
	}
}

func (m *MyService) MultiPle(r *http.Request) (int, error) {
	return 1, errors.New("Hi")
}

func (m *MyService) Products(r *http.Request) interface{} {

	return errors.New("Hello")
}

func (m *MyService) Products_0(r *http.Request, w http.ResponseWriter) interface{} {

	return "Product " + restruct.Param(r.Context(), "0") + " TAG: " + restruct.Param(r.Context(), "tag")
}

func main() {
	svc := restruct.NewHandler(&MyService{})
	svc.AddService("/{tag}/", &MyService{})
	restruct.Handle("/api/v1/", svc)
	port := "8090"
	log.Println("Listening " + port)
	http.ListenAndServe(":"+port, nil)
}

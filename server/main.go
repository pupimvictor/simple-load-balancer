package main

import (
	"net/http"
	"log"
	"github.com/pupimvictor/simple-load-balancer"

)


func main(){

	//read endpoints from input
	endpoints := []string{"http://localhost:9000", "http://localhost:9001", "http://localhost:9002"}
	//new lb
	lb, err := lb.NewLoadBalancer(endpoints)
	if err != nil {
		log.Fatalf("unable to start lb - %s", err.Error())
	}

	//http serve / to lb
	http.Handle("/", lb.Middleware(http.HandlerFunc(lb.Balancer)))
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

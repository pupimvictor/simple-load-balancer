package main

import (
	"github.com/pupimvictor/simple-load-balancer"
	"log"
	"net/http"

	"fmt"
	"net/url"
	"os"
)

var port = "8080"

func main() {

	//read endpoints from input
	var endpoints []*url.URL

	fmt.Printf("%s", os.Args)
	for i := 1; i < len(os.Args); i = i + 2 {
		if os.Args[i] == "-b" {
			url, err := url.Parse(os.Args[i+1])
			if err != nil {
				fmt.Errorf("invalid url input\n")
				return
			}
			endpoints = append(endpoints, url)
		} else if os.Args[i] == "-p" {
			port = os.Args[i+1]
		}
	}

	//new lb
	lb, err := lb.NewLoadBalancer(endpoints)
	if err != nil {
		log.Fatalf("unable to start lb - %s", err.Error())
	}

	go lb.StartHealthCheckJob()

	//http serve / to lb
	http.Handle("/", lb.Middleware(http.HandlerFunc(lb.Balancer)))
	err = http.ListenAndServe(":"+port, nil)
	if err != nil {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

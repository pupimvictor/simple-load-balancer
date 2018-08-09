package lb

import (
	"net/http"
	"log"

)


func main(){

	//read endpoints from input
	endpoints := []string{"http://localhost:9000", "http://localhost:9001"}
	//new lb
	lb, err := NewLoadBalancer(endpoints)
	if err != nil {
		log.Fatalf("unable to start lb - %s", err.Error())
	}

	//http serve / to lb
	http.HandleFunc("/", lb.Proxy)
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatalf("unable to start server: %s", err.Error())
	}
}

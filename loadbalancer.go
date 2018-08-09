package lb

import (
	"net/url"
	"net/http"
	"fmt"

	"strconv"
	"context"
)

type LoadBalancer struct{
	backends []ServerInstance
	healthyInstances *HealthyInstances
}

type ServerInstance struct{
	id int
	endpoint *url.URL
	healthy bool
}

type HealthyInstances struct {
	currInstance *ServerInstance
	nextInstance *HealthyInstances
}


func NewLoadBalancer(endpoints []string) (LoadBalancer, error){
	serverInstances := make([]ServerInstance, len(endpoints))

	for i, x := range endpoints{
		url, err := url.Parse(x)
		if err != nil {
			return LoadBalancer{}, fmt.Errorf("error parsing url: %v", err)
		}
		serverInstances[i] = ServerInstance{i, url, false}
	}

	lb := LoadBalancer{serverInstances, nil}
	lb.serverInstancesHealthCheck()

	return lb, nil
}

func (lb *LoadBalancer) Proxy(w http.ResponseWriter, req *http.Request) {
	//responsible to dispach the request to the backend
}

func (lb *LoadBalancer) BalancerMiddleware(h http.Handler) http.Handler{
	//responsible for defining the right backend and retry mechanism
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		reqStatus := 0
		serverInstance := lb.getNextHealthyServerInstance()
		activeInstance := &serverInstance
		firstInstanceId := activeInstance.currInstance.id
		for reqStatus != 200 {
			reqCtx := context.WithValue(req.Context(), "server-instance", activeInstance.currInstance)

			h.ServeHTTP(rw, req.WithContext(reqCtx))
			reqStatus, _ = strconv.Atoi(rw.Header().Get("status"))

			activeInstance = activeInstance.nextInstance
			if activeInstance.currInstance.id == firstInstanceId{
				break
			}
		}
	})
}

func (lb *LoadBalancer) LogMiddleware(h http.Handler) http.Handler{
	//logger middleware for request and balancing logs - don't know where to put yet
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		reqCtx := context.WithValue(ctx, "req-id", req.Header.Get("X-Request-ID"))
		h.ServeHTTP(rw, req.WithContext(reqCtx))
	})
}

func (lb *LoadBalancer) serverInstancesHealthCheck(){
	//call /_health for each one of the instances concurrently and update lb.Healthy instances
}

func (lb *LoadBalancer) getNextHealthyServerInstance() (HealthyInstances){

	healthyInstances := *lb.healthyInstances
	lb.healthyInstances = lb.healthyInstances.nextInstance

	return healthyInstances
}


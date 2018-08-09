package lb

import (
	"net/url"
	"net/http"
	"fmt"

	"context"
	"net/http/httputil"
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
	ctx := req.Context()
	serverInstance := ctx.Value("server-instance").(*ServerInstance)

	reverseProxy := httputil.NewSingleHostReverseProxy(serverInstance.endpoint)

	reverseProxy.ServeHTTP(w, req)
}

func (lb *LoadBalancer) BalancerMiddleware(h http.Handler) http.Handler{
	//responsible for defining the right backend and retry mechanism
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		reqStatus := 0
		serverInstance := lb.getNextHealthyServerInstance()
		activeInstance := &serverInstance
		firstInstanceId := activeInstance.currInstance.id

		scrw := NewStatusCodeResponseWriter(rw)

		for reqStatus != 200 {
			reqCtx := context.WithValue(req.Context(), "server-instance", activeInstance.currInstance)

			h.ServeHTTP(scrw, req.WithContext(reqCtx))
			reqStatus = scrw.statusCode

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

		//scrw := NewStatusCodeResponseWriter(rw)

		h.ServeHTTP(rw, req.WithContext(reqCtx))
	})
}

func (lb *LoadBalancer) serverInstancesHealthCheck(){
	//call /_health for each one of the instances concurrently and update lb.Healthy instances
	firstInst := &lb.backends[0]
	head := &HealthyInstances{firstInst, nil}
	prev := head
	for i := 1; i < len(lb.backends); i++ {
		inst := &lb.backends[i]
		curr := &HealthyInstances{inst, nil}
		prev.nextInstance = curr
		prev = curr
	}
	prev.nextInstance = head
	lb.healthyInstances = head
}

func (lb *LoadBalancer) getNextHealthyServerInstance() (HealthyInstances){

	healthyInstances := *lb.healthyInstances
	lb.healthyInstances = lb.healthyInstances.nextInstance

	return healthyInstances
}


type statusCodeResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (scrw *statusCodeResponseWriter) WriteHeader(code int) {
	scrw.statusCode = code
	scrw.ResponseWriter.WriteHeader(code)
}

func NewStatusCodeResponseWriter(w http.ResponseWriter) *statusCodeResponseWriter {
	return &statusCodeResponseWriter{w, http.StatusOK}
}

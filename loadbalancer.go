package lb

import (
	"fmt"
	"net/http"
	"net/url"

	"context"
	"net/http/httputil"
	"time"
)

type LoadBalancer struct {
	backends         []ServerInstance
	healthyInstances *HealthyInstances
}

type ServerInstance struct {
	id       int
	endpoint *url.URL
	healthy  bool
}

type HealthyInstances struct {
	currInstance *ServerInstance
	nextInstance *HealthyInstances
}

func NewLoadBalancer(endpoints []string) (LoadBalancer, error) {
	serverInstances := make([]ServerInstance, len(endpoints))

	for i, x := range endpoints {
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

func (lb *LoadBalancer) Balancer(rw http.ResponseWriter, req *http.Request) {
	//responsible for defining the right backend and retry mechanism
	serverInstance := lb.getNextHealthyServerInstance()
	activeInstance := &serverInstance
	firstInstanceId := activeInstance.currInstance.id


	scrw := rw.(*CustomResponseWriter)


	for {
		reverseProxy := httputil.NewSingleHostReverseProxy(activeInstance.currInstance.endpoint)
		reverseProxy.ServeHTTP(scrw, req)

		ctx := context.WithValue(req.Context(), "server-instance", activeInstance.currInstance)
		req = req.WithContext(ctx)

		if scrw.statusCode == 200 {
			break
		}

		activeInstance = activeInstance.nextInstance
		if activeInstance.currInstance.id == firstInstanceId {
			fmt.Printf("req #%s failed: all backends are sick\n\n", req.Context().Value("req-id"))
			break
		}
	}
	scrw.ResponseWriter.WriteHeader(scrw.statusCode)
	scrw.ResponseWriter.Write(scrw.body)

}

func (lb *LoadBalancer) Middleware(h http.Handler) http.Handler {
	//logger middleware for request and balancing logs - don't know where to put yet
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		start := time.Now()
		reqId := start.UnixNano() % 10000000000000

		ctx := req.Context()
		reqCtx := context.WithValue(ctx, "req-id", reqId)

		scrw := NewCustomResponseWriter(rw)

		h.ServeHTTP(scrw, req.WithContext(reqCtx))

		elapsed := time.Since(start)
		fmt.Printf("request # %d\nresponse: %d, elapsed: %s\n\n", reqId, scrw.statusCode, elapsed)
	})
}

func (lb *LoadBalancer) serverInstancesHealthCheck() {
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

func (lb *LoadBalancer) getNextHealthyServerInstance() HealthyInstances {

	healthyInstances := *lb.healthyInstances
	lb.healthyInstances = lb.healthyInstances.nextInstance

	return healthyInstances
}

type CustomResponseWriter struct {
	http.ResponseWriter
	statusCode int
	body       []byte
}

func (crw *CustomResponseWriter) Write(data []byte) (int, error) {
	crw.body = data
	return len(data), nil
}

func (crw *CustomResponseWriter) WriteHeader(code int) {
	crw.statusCode = code
}

func NewCustomResponseWriter(w http.ResponseWriter) *CustomResponseWriter {
	return &CustomResponseWriter{w, http.StatusOK, nil}
}

package lb

import (
	"fmt"
	"net/http"
	"net/url"

	"context"
	"golang.org/x/sync/errgroup"
	"log"
	"net/http/httputil"
	"time"

	"encoding/json"
	"io/ioutil"
	"math/rand"
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

type HealthCheckResp struct {
	State   string `json:"state"`
	Message string `json:"message"`
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

	go lb.startHealthCheckJob()

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

		if scrw.StatusCode == 200 {
			break
		}

		activeInstance = activeInstance.nextInstance
		if activeInstance.currInstance.id == firstInstanceId {
			fmt.Printf("request # %d\nfailed: all backends are sick\n", req.Context().Value("req-id"))
			break
		}
	}
	scrw.ResponseWriter.WriteHeader(scrw.StatusCode)
	scrw.ResponseWriter.Write(scrw.Body)

}

func (lb *LoadBalancer) Middleware(h http.Handler) http.Handler {
	//logger middleware for request and balancing logs - don't know where to put yet
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		start := time.Now()
		reqId := rand.Intn(100000)

		ctx := req.Context()
		reqCtx := context.WithValue(ctx, "req-id", reqId)

		scrw := NewCustomResponseWriter(rw)

		h.ServeHTTP(scrw, req.WithContext(reqCtx))

		elapsed := time.Since(start)
		fmt.Printf("request # %d\nresponse: %d, elapsed: %s\n\n", reqId, scrw.StatusCode, elapsed)
	})
}

func (lb *LoadBalancer) serverInstancesHealthCheck() {
	//call /_health for each one of the instances concurrently and update lb.Healthy instances
	fmt.Println("health checking")

	var wg errgroup.Group
	for _, x := range lb.backends {
		inst := x
		wg.Go(func() error {
			healthUrl := fmt.Sprintf("%s://%s/_health", inst.endpoint.Scheme, inst.endpoint.Host)
			resp, err := http.Get(healthUrl)
			if err != nil {
				fmt.Errorf("error pinging instance %s", inst.endpoint.Host)
			}
			defer resp.Body.Close()

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Errorf("error reading health response")
			}

			var instHealth HealthCheckResp
			err = json.Unmarshal(body, &instHealth)
			if err != nil {
				fmt.Errorf("error unmarshal health response %v", err)
			}

			lb.backends[inst.id].healthy = "healthy" == instHealth.State

			fmt.Printf("instance %s is %s\n", inst.endpoint.Host, instHealth.State)
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		log.Fatalf("error health checking: %v", err)
	}

	lb.buildHealthyList()
}

func (lb *LoadBalancer) buildHealthyList() {
	var firstInst *ServerInstance
	for _, x := range lb.backends {
		if x.healthy {
			firstInst = &x
			break
		}
	}

	if firstInst == nil {
		time.Sleep(2 * time.Second)
		lb.buildHealthyList()
		return
	}

	head := &HealthyInstances{firstInst, nil}
	prev := head
	for i := 1; i < len(lb.backends); i++ {
		inst := &lb.backends[i]
		if !inst.healthy {
			continue
		}
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

func (lb *LoadBalancer) startHealthCheckJob() {
	tick := time.Tick(10 * time.Second)
	for range tick {
		lb.serverInstancesHealthCheck()
	}
}

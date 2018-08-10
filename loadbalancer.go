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

const healthCheckInterval = 10

//This function creates a LoadBalancer instance using a ServerInstance for each one of the input endpoints.
//The function calls lb.serverInstancesHealthCheck() that performs the health check for all the serverInstances and creates and assign to the LB a linked list HealthyInstances with the healthy serverInstances.
//The function concurrently call the healthCheckJob that will perform the health check process on the interval defined by `const healthCheckInterval`
func NewLoadBalancer(endpoints []string) (LoadBalancer, error) {
	fmt.Printf("%s", endpoints)
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

//vvvv Load Balancer Logic vvvv

//The Balancer  will call the getNextHealthyServerInstance to get the current serverInstance that should receive the request.
// getNextHealthyServerInstance will return a copy of the current node in the linked list.
// I've implemented this way so that any other requests or the health check job updating the linked list would not interfere on this request retries.
func (lb *LoadBalancer) Balancer(rw http.ResponseWriter, req *http.Request) {

	//Get the currentInstance and keep the reference in firstInstanceId to break the loop in case of all the requests are degraded
	serverInstance := lb.getNextHealthyServerInstance()
	activeInstance := &serverInstance
	firstInstanceId := activeInstance.currInstance.id

	//asserting rw as a customResponseWriter
	crw := rw.(*CustomResponseWriter)

	//This loop uses a ResverseProxy to redirect the request to the proper serverInstance
	//In the case of success (200) loop break and the redirect was successful
	//Other wise it will walk the serverInstances linked list and retry. If it retry on all the ServerInstances without success the loop break and the redirect was failed
	for {
		reverseProxy := httputil.NewSingleHostReverseProxy(activeInstance.currInstance.endpoint)
		reverseProxy.ServeHTTP(crw, req)

		ctx := context.WithValue(req.Context(), "server-instance", activeInstance.currInstance)
		req = req.WithContext(ctx)

		fmt.Printf("request # %d, ep: %s\n", ctx.Value("req-id"), activeInstance.currInstance.endpoint)
		if crw.StatusCode == 200 {
			break
		}

		activeInstance = activeInstance.nextInstance
		if activeInstance.currInstance.id == firstInstanceId {
			fmt.Printf("request # %d\nfailed: all backends are sick\n", req.Context().Value("req-id"))
			break
		}
	}
	//This is where the CustomResponseWriter comes in place:
	//When the reverseProxy serves the request to an instance, the response is written. If it is a fail, we want to retry, but it's not possible if the regular responseWriter has already written the response on the previous try.
	//To solve this problem I've created the CustomResponseWriter that postpone the write operation, saving it in a local variable and then actually writing it to the response only after the loop breaks. see respwriter.go:
	crw.WriteResponse()

}

//This middleware creates a random request id and logs the elapsed time.
//It also creates a CustomResponseWriter that is a implementation of the ResponseWriter interface that will allow balancer to retry the request using the same writer.
func (lb *LoadBalancer) Middleware(h http.Handler) http.Handler {
	//logger middleware for request and balancing logs - don't know where to put yet
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		start := time.Now()
		reqId := rand.Intn(100000)

		ctx := req.Context()
		reqCtx := context.WithValue(ctx, "req-id", reqId)

		//see respwriter.go
		scrw := NewCustomResponseWriter(rw)

		h.ServeHTTP(scrw, req.WithContext(reqCtx))

		elapsed := time.Since(start)
		fmt.Printf("request # %d, response: %d, elapsed: %s\n\n", reqId, scrw.StatusCode, elapsed)
	})
}


//vvvv Health Check Logic vvvv

//Get the next instance to balance the traffic
//The strategy implemented is a simple Round Robin.
//Important to notice that every time this method is called, it returns a copy of the current node of the linked list.
// That's important to isolate every request with it's own control of the linked list, making it simpler to create a retry mechanism and avoid loosing references when the healthcheck refresh the healthyInstances list
func (lb *LoadBalancer) getNextHealthyServerInstance() HealthyInstances {

	healthyInstances := *lb.healthyInstances
	lb.healthyInstances = lb.healthyInstances.nextInstance

	return healthyInstances
}

//Health check logic
//The serverInstancesHealthCheck function calls /_health for each one of the instances concurrently and save the response back to the ServerInstance healty attribute on the lb.backend list
//then update lb.HealthyInstances by calling the lb.buildHealthyList()
func (lb *LoadBalancer) serverInstancesHealthCheck() {

	fmt.Println("health checking")

	var wg errgroup.Group
	for _, x := range lb.backends {
		inst := x
		wg.Go(func() error {
			healthUrl := fmt.Sprintf("%s/_health", inst.endpoint)
			resp, err := http.Get(healthUrl)
			if err != nil {
				fmt.Printf("error pinging instance %s, %v", inst.endpoint.Host, err)
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

//This function traverses the lb.backends list filtering the healthy ServerInstances and creating a circular linked list
func (lb *LoadBalancer) buildHealthyList() {
	//checks for the first healthy ServerInstance to be the first element of the list
	var firstInst *ServerInstance
	for _, x := range lb.backends {
		if x.healthy {
			firstInst = &x
			break
		}
	}

	//if there's no healthy instance, the process restarts after 1 sec sleep
	if firstInst == nil {
		time.Sleep(1 * time.Second)
		lb.serverInstancesHealthCheck()
		return
	}

	//creates a circular list with the healthy instances
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

//cron job to health check
func (lb *LoadBalancer) startHealthCheckJob() {
	tick := time.Tick(healthCheckInterval * time.Second)
	for range tick {
		lb.serverInstancesHealthCheck()
	}
}

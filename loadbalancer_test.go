package lb

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLoadBalancer(t *testing.T){
	tests := []struct{
		name string
		nClients int
		nReqs int
		sickClients []bool
		turnSickAt  map[int]int
		expectedRes []string
	}{
		{
			name: "test1",
			nClients: 4,
			nReqs: 12,
			sickClients: []bool{false, false, true, false},
			turnSickAt: map[int]int{6:1},
			expectedRes: []string{"0", "1", "3", "0", "1", "3", "0", "3", "0", "3", "0", "3", "0"},
		},
	}


	for _, test := range tests{

		var endpoints []*url.URL
		for i := 0 ; i < test.nClients; i++{
			cli := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "_health") {
					//fmt.Errorf("%d iiii\n", i)
					if  false {
						fmt.Fprint(w, "{\"state\":\"sick\"}")
					}else{
						fmt.Fprint(w, "{\"state\":\"healthy\"}")
					}
				}else{
					fmt.Fprint(w, "{\"message\":\"success\"}")
					w.WriteHeader(200)
				}
			}))
			url, _ := url.Parse(cli.URL)
			endpoints = append(endpoints, url)
		}

		t.Run(test.name, func(ts *testing.T){

			fmt.Printf("%s", endpoints)
			serverInstances := make([]ServerInstance, len(endpoints))

			for i, x := range endpoints {
				serverInstances[i] = ServerInstance{i, x, false}
			}

			lbTest := LoadBalancer{serverInstances, nil, func(inst ServerInstance, healthErr []error) (HealthCheckResp, []error) {
				return HealthCheckResp{"healthy", "oi"}, nil
			}}
			err := lbTest.serverInstancesHealthCheck()
			if err != nil {
				ts.Errorf("errors during health check: %v", err)
			}

			rr := httptest.NewRecorder()

			handler := http.HandlerFunc(lbTest.Balancer)

			for i := 0 ; i < test.nReqs ; i++{
				req, err := http.NewRequest("GET", "/", nil)
				if err != nil {
					ts.Errorf("error new request: %v", err)
				}
				handler.ServeHTTP(rr, req)

				if string(rr.Body.Bytes()) != test.expectedRes[i] {
					ts.Errorf("espected resp %s got %d", test.expectedRes, rr.Code)
				}
			}
		})
	}
}

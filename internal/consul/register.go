package consul

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

var controllerTags = []string{
	"traefik.enable=true",
	"traefik.http.routers.controller.rule=Host(`api.universidad.localhost`)",
	"traefik.http.routers.controller.entryPoints=https",
	"traefik.http.routers.controller.tls=true",
	"traefik.http.services.controller.loadbalancer.server.port=5000",
	"traefik.http.middlewares.bulkhead-controller.inflightreq.amount=50",
	"traefik.http.routers.controller.middlewares=bulkhead-controller",
}

type serviceRegistration struct {
	ID      string      `json:"ID"`
	Name    string      `json:"Name"`
	Address string      `json:"Address"`
	Port    int         `json:"Port"`
	Tags    []string    `json:"Tags"`
	Check   healthCheck `json:"Check"`
}

type healthCheck struct {
	HTTP                            string `json:"HTTP"`
	Interval                        string `json:"Interval"`
	Timeout                         string `json:"Timeout"`
	DeregisterCriticalServiceAfter  string `json:"DeregisterCriticalServiceAfter"`
}

// RegisterController registers this controller instance with Consul in a background goroutine.
func RegisterController(consulURL string, port int) {
	go registerWithRetry(consulURL, port)
}

func registerWithRetry(consulURL string, port int) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "controller"
	}

	// Prefer the container's first non-loopback IP so Consul health checks can reach us
	addr := resolveIP(hostname)

	payload := serviceRegistration{
		ID:      fmt.Sprintf("controller-%s", hostname),
		Name:    "controller",
		Address: addr,
		Port:    port,
		Tags:    controllerTags,
		Check: healthCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health", addr, port),
			Interval:                       "15s",
			Timeout:                        "5s",
			DeregisterCriticalServiceAfter: "30s",
		},
	}

	body, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 5 * time.Second}

	for attempt := range 10 {
		resp, err := client.Do(newPutRequest(consulURL+"/v1/agent/service/register", body))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("consul: registered as %s", payload.ID)
				return
			}
			log.Printf("consul: registration returned HTTP %d", resp.StatusCode)
		} else {
			log.Printf("consul: registration attempt %d failed: %v", attempt+1, err)
		}
		time.Sleep(5 * time.Second)
	}

	log.Printf("consul: failed to register after 10 attempts")
}

func newPutRequest(url string, body []byte) *http.Request {
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// resolveIP returns the first non-loopback IPv4 address for the given hostname,
// falling back to the hostname itself if resolution fails.
func resolveIP(hostname string) string {
	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return hostname
	}
	for _, a := range addrs {
		if ip := net.ParseIP(a); ip != nil && !ip.IsLoopback() {
			return a
		}
	}
	return hostname
}

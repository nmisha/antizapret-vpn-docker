package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Route struct {
	Hostname   string `json:"hostname"`
	IpExternal net.IP `json:"ip_external"`
	IpInternal net.IP `json:"ip_internal"`
}

func main() {
	outputPath := flag.String("output", "/root/antizapret/result/dns.txt", "Path to the output DNS file")
	interval := flag.Duration("interval", 5*time.Second, "Interval between DNS updates")
	flag.Parse()

	routesRaw := os.Getenv("ROUTES")
	if routesRaw == "" {
		fmt.Println("ROUTES environment variable is not set")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	routes := parseRoutes(routesRaw)

	for {
		select {
		case <-time.After(*interval):
			if updateDns(routes) {
				fmt.Println("DNS updated, writing to file...")
				if jsonData, err := json.MarshalIndent(routes, "", "  "); err == nil {
					os.WriteFile(*outputPath, []byte(jsonData), 0644)
				}
			}
		case <-ctx.Done():
			fmt.Println("Gracefully shutting down...")
			return
		}
	}

}

func parseRoutes(routesStr string) map[string]Route {
	fmt.Printf("Raw ROUTES: %s\n", routesStr)

	// Clean up and split the string
	routesStr = strings.TrimSpace(routesStr)
	parts := strings.Split(routesStr, ";")

	routes := make(map[string]Route)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			routes[key] = Route{
				Hostname:   key,
				IpInternal: net.ParseIP(value),
			}
		}
	}

	fmt.Println("Parsed Routes:")
	for key, value := range routes {
		fmt.Printf("  %s: %s\n", key, value)
	}

	return routes
}

func updateDns(routes map[string]Route) bool {
	changed := false
	for key, route := range routes {
		// DNS Query
		fmt.Printf("Querying DNS for %s (internal IP: %s)\n", key, route.IpInternal)
		if ip, err := net.LookupIP(route.Hostname); err != nil {
			fmt.Printf("DNS lookup failed for %s: %v\n", route.Hostname, err)
		} else {
			if !route.IpExternal.Equal(ip[0]) {
				fmt.Printf("External IP for %s changed: %s -> %s\n", route.Hostname, route.IpExternal, ip[0])
				route.IpExternal = ip[0]
				routes[key] = route
				changed = true
			}
		}
	}

	return changed
}

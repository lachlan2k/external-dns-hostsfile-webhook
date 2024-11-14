package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

type HostsfilesProvider struct {
	path      string
	hostToIp  map[string]string
	ipToHosts map[string][]string
	lock      sync.Mutex
}

// Caller must hold lock
func (h *HostsfilesProvider) insert(ip string, hosts []string) {
	existingHosts, ok := h.ipToHosts[ip]
	if ok && len(existingHosts) > 0 {
		h.ipToHosts[ip] = append(existingHosts, hosts...)
	} else {
		h.ipToHosts[ip] = hosts
	}

	for _, host := range hosts {
		h.hostToIp[host] = ip
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) removeByHost(host string) {
	ip := h.hostToIp[host]
	delete(h.hostToIp, host)

	h.ipToHosts[ip] = slices.DeleteFunc(h.ipToHosts[ip], func(s string) bool {
		return s == host
	})
	if len(h.ipToHosts[ip]) == 0 {
		delete(h.ipToHosts, ip)
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) loadFromDisk() {
	h.hostToIp = make(map[string]string)
	h.ipToHosts = make(map[string][]string)

	file, err := os.Open(h.path)
	if err != nil {
		log.Printf("Warn: failed to open hosts file for reading: %v\n", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			log.Printf("Warn: found unvalid line in hosts files: %q\n", line)
			continue
		}

		ip := parts[0]
		hosts := parts[1:]

		h.insert(ip, hosts)
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) writeToDisk() {
	file, err := os.Create(h.path)
	if err != nil {
		log.Printf("Warn: failed to open hosts file for writing: %v\n", err)
		return
	}
	defer file.Close()

	for ip, hosts := range h.ipToHosts {
		fmt.Fprintf(file, "%s %s\n", ip, strings.Join(hosts, " "))
	}
}

const ttl = 10

var labels = map[string]string{}

func (h *HostsfilesProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.loadFromDisk()

	records := []*endpoint.Endpoint{}
	for host, ip := range h.hostToIp {
		records = append(records, &endpoint.Endpoint{
			DNSName:          host,
			Targets:          []string{ip},
			RecordType:       "A",
			SetIdentifier:    "", // TODO: do we need to store this? If so it could be a comment?
			RecordTTL:        ttl,
			Labels:           labels,
			ProviderSpecific: nil,
		})
	}

	return records, nil
}

func (h *HostsfilesProvider) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	defer h.writeToDisk()

	for _, toCreate := range changes.Create {
		log.Printf("Creating endpoint %q\n", toCreate.String())

		if len(toCreate.Targets) == 0 {
			log.Printf("Endpoint contained no targets")
			continue
		}
		if toCreate.RecordType != "A" {
			log.Printf("Only A records are supported, received %q", toCreate.RecordType)
			continue
		}

		h.insert(toCreate.Targets[0], []string{toCreate.DNSName})
	}

	for _, toDelete := range changes.Delete {
		log.Printf("Deleting endpoint %q\n", toDelete.String())
		h.removeByHost(toDelete.DNSName)
	}

	for i, old := range changes.UpdateOld {
		log.Printf("Removing existing endpoint %d for update %q\n", i, old.String())
		h.removeByHost(old.DNSName)
	}

	for i, toUpdate := range changes.UpdateNew {
		log.Printf("Updating endpoint %d for update %q\n", i, toUpdate.String())
		if toUpdate.RecordType != "A" {
			log.Printf("Only A records are supported, received %q", toUpdate.RecordType)
			continue
		}

		h.insert(toUpdate.Targets[0], []string{toUpdate.DNSName})
	}
	return nil
}

func (h *HostsfilesProvider) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	for _, endpoint := range endpoints {
		endpoint.RecordTTL = ttl
		endpoint.Labels = labels
	}

	return endpoints, nil
}

type domainFilter struct{}

func (domainFilter) Match(domain string) bool {
	return true
}

func (h *HostsfilesProvider) GetDomainFilter() endpoint.DomainFilterInterface {
	return &domainFilter{}
}

func main() {
	hostsFileP := flag.String("hosts-file", "", "Path to the hosts file to update")
	flag.Parse()

	if *hostsFileP == "" {
		log.Printf("No hosts file path provided")
		flag.Usage()
		return
	}

	provider := &HostsfilesProvider{
		path: *hostsFileP,
	}

	provider.loadFromDisk()

	p := api.WebhookServer{
		Provider: provider,
	}
	m := http.NewServeMux()
	m.HandleFunc("/", p.NegotiateHandler)
	m.HandleFunc("/records", p.RecordsHandler)
	m.HandleFunc("/adjustendpoints", p.AdjustEndpointsHandler)
	m.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	fmt.Printf("Listening on :8888 and :8080 for hostfile %s\n", provider.path)
	go func() {
		log.Fatal(http.ListenAndServe(":8080", m))
	}()
	log.Fatal(http.ListenAndServe(":8888", m))
}

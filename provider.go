package main

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type HostsfilePersister interface {
	Read() (string, error)
	Write(string) error
}

type HostsfilesProvider struct {
	hostToIPs map[string][]string

	lock      sync.Mutex
	persister HostsfilePersister
}

// Caller must hold lock
func (h *HostsfilesProvider) insert(host string, ip string) {
	existingIPs, ok := h.hostToIPs[host]
	if ok && len(existingIPs) > 0 {
		h.hostToIPs[host] = append(existingIPs, ip)
	} else {
		h.hostToIPs[host] = []string{ip}
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) remove(host string, ip string) {
	h.hostToIPs[host] = slices.DeleteFunc(h.hostToIPs[host], func(s string) bool {
		return s == ip
	})
	if len(h.hostToIPs[host]) == 0 {
		delete(h.hostToIPs, host)
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) load() {
	h.hostToIPs = make(map[string][]string)

	contents, err := h.persister.Read()
	if err != nil {
		log.Printf("Warn: failed to read hosts file: %v\n", err)
		return
	}

	lines := strings.Split(contents, "\n")
	for _, line := range lines {
		line := strings.TrimSpace(line)
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

		for _, host := range hosts {
			h.insert(host, ip)
		}
	}
}

// Caller must hold lock
func (h *HostsfilesProvider) persist() {
	out := ""

	for host := range h.hostToIPs {
		ips := h.hostToIPs[host]
		for _, ip := range ips {
			out += fmt.Sprintf("%s %s\n", ip, host)
		}
	}

	err := h.persister.Write(out)
	if err != nil {
		log.Printf("Warn: failed to write hosts file: %v\n", err)
		return
	}
}

const ttl = 10

var labels = map[string]string{}

func (h *HostsfilesProvider) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	h.lock.Lock()
	defer h.lock.Unlock()

	h.load()

	records := []*endpoint.Endpoint{}
	for host, ips := range h.hostToIPs {
		records = append(records, &endpoint.Endpoint{
			DNSName:          host,
			Targets:          ips,
			RecordType:       "A",
			SetIdentifier:    "",
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

	defer h.persist()

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

		for _, target := range toCreate.Targets {
			h.insert(toCreate.DNSName, target)
		}
	}

	for _, toDelete := range changes.Delete {
		log.Printf("Deleting endpoint %q\n", toDelete.String())
		for _, target := range toDelete.Targets {
			h.remove(toDelete.DNSName, target)
		}
	}

	// We get UpdateOld of what to remove and UpdateNew of what to add.
	for i, old := range changes.UpdateOld {
		log.Printf("Removing existing endpoint %d for update %q\n", i, old.String())
		for _, target := range old.Targets {
			h.remove(old.DNSName, target)
		}
	}
	for i, toUpdate := range changes.UpdateNew {
		log.Printf("Updating endpoint %d for update %q\n", i, toUpdate.String())
		if toUpdate.RecordType != "A" {
			log.Printf("Only A records are supported, received %q", toUpdate.RecordType)
			continue
		}

		for _, target := range toUpdate.Targets {
			h.insert(toUpdate.DNSName, target)
		}
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

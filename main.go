package main

import (
	"flag"
	"log"
	"net/http"

	"sigs.k8s.io/external-dns/provider/webhook/api"
)

func main() {
	hostsFileP := flag.String("hosts-file", "", "Path to the hosts file to update")
	backendP := flag.String("backend", "disk", "Backend to persist hostsfile, options: disk, configmap")
	configMapNamespaceP := flag.String("configmap-namespace", "default", "Namespace for the configmap backend")
	configMapNameP := flag.String("configmap-name", "external-dns-hostsfile", "Name of the configmap to use for the configmap backend")

	flag.Parse()

	var persister HostsfilePersister

	if *backendP == "disk" {
		if *hostsFileP == "" {
			log.Fatal("You must provide a path to the hosts file when using the disk backend")
		}

		persister = &OnDiskHostsfilePersister{
			path: *hostsFileP,
		}
	} else if *backendP == "configmap" {
		if *configMapNamespaceP == "" || *configMapNameP == "" {
			log.Fatal("You must provide a namespace and name for the configmap when using the configmap backend")
		}

		var err error
		persister, err = NewConfigMapHostsfilePersister(*configMapNamespaceP, *configMapNameP)
		if err != nil {
			log.Fatalf("Error creating ConfigMap persister: %v", err)
		}
	} else {
		log.Fatalf("Unknown backend %s, supported backends are: disk, configmap", *backendP)
	}

	provider := &HostsfilesProvider{
		persister: persister,
	}

	provider.load()

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

	log.Println("Listening on :8888 and :8080")
	go func() {
		log.Fatal(http.ListenAndServe(":8080", m))
	}()
	log.Fatal(http.ListenAndServe(":8888", m))
}

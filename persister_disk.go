package main

import (
	"os"
)

type OnDiskHostsfilePersister struct {
	path string
}

func (p *OnDiskHostsfilePersister) Read() (string, error) {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (p *OnDiskHostsfilePersister) Write(contents string) error {
	return os.WriteFile(p.path, []byte(contents), 0644)
}

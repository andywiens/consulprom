package main

import (
	"context"
	"crypto/md5"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/hashicorp/consul/api"
)

// ConfigState holds the configuration and its MD5 sum
type ConfigState struct {
	Config string
	Md5Sum string
}

type queryResult struct {
	Err  error
	Pair *api.KVPair
}

// Continually longpoll's consul looking for an entry with a more recent modifyindex
func waitForChanges(ctx context.Context, keyPath string, changeQ chan<- struct{}, startIndex uint64, client *api.Client) {
	oldIdx := startIndex
	for {
		select {
		case <-ctx.Done():
			return
		case res := <-blockingKeyCheck(oldIdx, keyPath, client):
			if res.Err != nil {
				fmt.Printf("Error polling consul: %v\n", res.Err)
				time.Sleep(1 * time.Minute)
			} else if res.Pair.ModifyIndex > oldIdx {
				oldIdx = res.Pair.ModifyIndex
				go func() {
					fmt.Println("reporting change found during longpoll")
					select {
					case changeQ <- struct{}{}:
					case <-ctx.Done():
					}
				}()
			} else {
				fmt.Println("Nothing new during polling interval")
			}
		}
	}
}

// Tries to get the key from consul using longpoll. The value is written to the returned channel
func blockingKeyCheck(index uint64, keyPath string, client *api.Client) <-chan *queryResult {
	resultQ := make(chan *queryResult)

	go func() {
		defer close(resultQ)
		res := &queryResult{}
		opts := &api.QueryOptions{}
		opts.WaitIndex = index
		opts.WaitTime = 55 * time.Second
		res.Pair, _, res.Err = client.KV().Get(keyPath, opts)
		resultQ <- res
	}()
	return resultQ
}

// replace the file if it has changed
func manageReplacement(keyPath, filePath string, client *api.Client) (bool, error) {
	existing := existingConfig(filePath)
	latest, err := findConfig(keyPath, client)
	if err != nil {
		return false, fmt.Errorf("Invalid template in consul: %v", err)
	}
	// fmt.Printf("existing: %s; latest: %s\n", existing.Md5Sum, latest.Md5Sum)
	if existing.Md5Sum == latest.Md5Sum {
		return false, nil
	}

	if err := writeConfig(filePath, latest); err != nil {
		return false, err
	}

	return true, nil
}

// find the configuration in Consul
func findConfig(lookupKey string, client *api.Client) (*ConfigState, error) {

	// Get a handle to the KV API
	kv := client.KV()

	// Lookup the pair
	pair, _, err := kv.Get(lookupKey, nil)
	if err != nil {
		panic(err)
	}
	if pair == nil {
		fmt.Printf("Value not found: %s\n", lookupKey)
		return nil, nil
	}

	processed, err := processAsTemplate(string(pair.Value))
	if err != nil {
		fmt.Println("invalid template!")
		return nil, err
	}
	return &ConfigState{
		Config: processed,
		Md5Sum: fmt.Sprintf("%x", md5.Sum([]byte(processed))),
	}, nil
}

// Write the configurtion to the file (defined by path)
func writeConfig(path string, config *ConfigState) error {
	return ioutil.WriteFile(path, []byte(fmt.Sprintf("# MD5: %s\n%s", config.Md5Sum, config.Config)), os.FileMode(0664))
}

// reads existing configuration from the filesystem
// Makes assumptions that it's parsing a file of the expected format... no error checking at this point
func existingConfig(path string) *ConfigState {
	conf, err := ioutil.ReadFile(path)
	cs := &ConfigState{}
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("missing configuration... will just create a new one")
			return cs
		}
		panic(fmt.Sprintf("cannot open config: %v", err))
	}

	parts := strings.Split(string(conf), "\n")
	cs.Config = strings.Join(parts[1:], "\n")
	cs.Md5Sum = parseMarkerTag(parts[0])
	return cs
}

// parses the md5 sum from "# MD5: xxxxxxxxx"
func parseMarkerTag(line string) string {
	vals := Md5TagRx.FindStringSubmatch(line)
	if vals == nil {
		return ""
	}
	return vals[1]
}

func processAsTemplate(data string) (string, error) {
	funcs := template.FuncMap{
		"env": os.Getenv,
	}

	tmpl, err := template.New("config").Funcs(funcs).Parse(data)
	if err != nil {
		return "", err
	}

	var processed strings.Builder
	tmpl.Execute(&processed, struct{}{})
	return (&processed).String(), nil
}

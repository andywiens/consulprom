package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"syscall"
	"time"

	"github.com/hashicorp/consul/api"
)

var (
	Md5TagRx = regexp.MustCompile("MD5: (\\w+)")
)

func main() {

	conf := mustGetConf()

	monitorQ := make(chan struct{})
	reloadQ := make(chan int)
	checkInterval := 50 * time.Minute

	// Get a new consul client
	consulConf := api.DefaultConfig()
	consulConf.Address = conf.ConsulAddr
	client, err := api.NewClient(consulConf)
	if err != nil {
		panic(err)
	}

	if _, err := manageReplacement(conf.ConsulKey, conf.AppConfigPath, client); err != nil {
		panic(fmt.Sprintf("Failed loading the config file!: %v\n", err))
	}

	app := start(conf.App, os.Args[1:])

	ctx, cancelFunc := context.WithCancel(context.Background())
	go monitorUpdates(ctx, monitorQ, reloadQ, checkInterval, conf.ConsulKey, conf.AppConfigPath, client)
	go manageReloads(ctx, reloadQ, app)
	go waitForChanges(ctx, conf.ConsulKey, monitorQ, 0, client)

	if err := app.Wait(); err != nil {
		fmt.Printf("Application exited with error: %v\n", err)
	}

	fmt.Println("Application exited")
	time.Sleep(5 * time.Second)
	cancelFunc()
	// TODO: put in a graceful quit and forward SIGHUP & SIGINT

}

func monitorUpdates(ctx context.Context, inchan <-chan struct{}, reloadQ chan<- int, period time.Duration, keyPath, filePath string, client *api.Client) {
	for {
		select {
		case <-time.After(period):
			replaceConfig(keyPath, filePath, reloadQ, client)
		case <-inchan:
			replaceConfig(keyPath, filePath, reloadQ, client)
		case <-ctx.Done():
			return
		}
	}
}

func replaceConfig(keyPath, filePath string, reloadQ chan<- int, client *api.Client) {
	if ok, err := manageReplacement(keyPath, filePath, client); ok {
		go func() { reloadQ <- 1 }()
	} else if err != nil {
		fmt.Printf("ERROR replacing config (%s): %v\n", filePath, err)
	}
}

func manageReloads(ctx context.Context, inchan <-chan int, app *exec.Cmd) {
	for {
		select {
		case <-inchan:
			err := reloadApp(app)
			if err != nil {
				fmt.Printf("ERROR reloading app (%s): %v", app.Path, err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func reloadApp(cmd *exec.Cmd) error {
	return cmd.Process.Signal(syscall.SIGHUP)
}

func start(app string, args []string) *exec.Cmd {
	cmd := exec.Command(app, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	return cmd
}

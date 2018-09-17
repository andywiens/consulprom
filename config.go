package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"gopkg.in/yaml.v2"
)

type Config struct {
	App           string `yaml:"app"`
	AppConfigPath string `yaml:"appConfigPath"`
	ConsulKey     string `yaml:"consulKey"`
	ConsulAddr    string `yaml:"consulAddr"`
}

var (
	ConsulAddrEnvName = "CONSUL_ADDR"
	DefaultConfigFile = "consulprom.yml"
)

// return path to default config
func defaultConfig() string {
	pwd, _ := os.Getwd()
	return path.Join(pwd, DefaultConfigFile)
}

func mustGetConf() *Config {
	conf := &Config{}
	yamlFile, err := ioutil.ReadFile(defaultConfig())
	if err != nil {
		panic(fmt.Sprintf("Error reading config: %v\n", err))
	}
	err = yaml.Unmarshal(yamlFile, conf)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse config: %v", err))
	}

	if conf.ConsulAddr == "" {
		conf.ConsulAddr = os.Getenv(ConsulAddrEnvName)
	}

	return conf
}

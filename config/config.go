package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

// Config is the main Configuration Struct for ZASentinel.
type Config struct {
	Path      string    `yaml:"path,omitempty"`
	Interface Interface `yaml:"interface"`
	Peers     []string  `yaml:"peers"`
}

// Interface defines all of the fields that a local node needs to know about itself!
type Interface struct {
	Name       string `yaml:"name"`
	ID         string `yaml:"id"`
	ListenPort int    `yaml:"listen_port"`
	PrivateKey string `yaml:"private_key"`
}

// Read initializes a config from a file.
func Read(path string) (*Config, error) {
	in, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	result := Config{
		Interface: Interface{
			Name:       "hs0",
			ListenPort: 8001,
			ID:         "",
			PrivateKey: "",
		},
	}

	// Read in config settings from file.
	err = yaml.Unmarshal(in, &result)
	if err != nil {
		return nil, err
	}

	// Overwrite path of config to input.
	result.Path = path
	return &result, nil
}

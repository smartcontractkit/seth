package test_utils

import (
	"github.com/pelletier/go-toml/v2"
	"github.com/smartcontractkit/seth"
)

func CopyConfig(config *seth.Config) (*seth.Config, error) {
	marshalled, err := toml.Marshal(config)
	if err != nil {
		return nil, err
	}

	var configCopy seth.Config
	err = toml.Unmarshal(marshalled, &configCopy)
	if err != nil {
		return nil, err
	}

	return &configCopy, nil
}

package main

import (
	"errors"
	"fmt"
	"strings"
)

type RootConfig struct {
	Streams  map[string]StreamConfig `yaml:"streams" json:"streams"`
	Ingester IngesterConfig          `yaml:"ingester" json:"ingester"`
}

func (this *RootConfig) Valid() error {

	if len(this.Streams) == 0 {
		return errors.New("no streams defined")
	}

	for key, item := range this.Streams {

		if err := item.Valid(); err != nil {
			return fmt.Errorf("error validating stream '%s' config: %s", key, err.Error())
		}

		item.ID = key
		this.Streams[key] = item
	}

	if err := this.Ingester.Valid(); err != nil {
		return fmt.Errorf("error validating ingester config: %s", err.Error())
	}

	return nil
}

type StreamConfig struct {
	ID     string
	Name   string            `yaml:"name" json:"name"`
	Labels map[string]string `yaml:"labels" json:"labels"`
}

func (this *StreamConfig) Valid() error {
	this.Name = strings.TrimSpace(this.Name)
	return nil
}

type IngesterConfig struct {
	KeepEmptyLabels bool `yaml:"keep_empty_labels" json:"keep_empty_labels"`
	MaxPayloadSize  int  `yaml:"max_payload_size" json:"max_payload_size"`
}

func (this *IngesterConfig) Valid() error {

	if this.MaxPayloadSize == 0 {
		this.MaxPayloadSize = 10_000_000
	}

	if this.MaxPayloadSize < 0 || (this.MaxPayloadSize > 0 && this.MaxPayloadSize < 1_000) {
		return errors.New("payload size cannot be smaller than 1KB")
	}

	return nil
}

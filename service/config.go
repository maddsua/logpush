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
	Tag    string            `yaml:"tag" json:"tag"`
	Token  string            `yaml:"token" json:"token"`
	Labels map[string]string `yaml:"labels" json:"labels"`
}

func (this *StreamConfig) Valid() error {

	if this.Tag = strings.TrimSpace(this.Tag); this.Tag == "" {
		return errors.New("stream tag must not be empty")
	}

	this.Token = strings.TrimSpace(this.Token)

	return nil
}

type IngesterConfig struct {
	KeepEmptyLabels bool `yaml:"keep_empty_labels" json:"keep_empty_labels"`
	MaxPayloadSize  int  `yaml:"max_payload_size" json:"max_payload_size"`
	MaxMessageSize  int  `yaml:"max_message_size" json:"max_message_size"`
	MaxLabelSize    int  `yaml:"max_label_size" json:"max_label_size"`
}

func (this *IngesterConfig) Valid() error {

	if this.MaxPayloadSize == 0 {
		this.MaxPayloadSize = 10_000_000
	}

	if this.MaxPayloadSize < 0 || (this.MaxPayloadSize > 0 && this.MaxPayloadSize < 1_000) {
		return errors.New("max payload size cannot be smaller than 1KB")
	}

	if this.MaxMessageSize <= 0 {
		this.MaxMessageSize = 10_000
	} else if this.MaxMessageSize < 100 {
		return errors.New("max message size cannot be smaller than 100 symbols")
	}

	if this.MaxLabelSize <= 0 {
		this.MaxLabelSize = 50
	} else if this.MaxLabelSize < 10 {
		return errors.New("max message size cannot be smaller than 10 symbols")
	}

	return nil
}

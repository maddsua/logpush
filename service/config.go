package main

type RootConfig struct {
	Streams map[string]StreamConfig `yaml:"streams"`
}

type StreamConfig struct {
	ID     string
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels"`
}

type IngesterConfig struct {
	MaxLabels       int
	MaxLabelNameLen int
	MaxLabelLen     int
	MaxMessages     int
	MaxMessageLen   int
	KeepEmptyLabels bool
}

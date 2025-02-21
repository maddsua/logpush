package loki

import (
	"log/slog"
)

func filterLabelFormat(labels map[string]string) {

	for key, value := range labels {

		if stripped := stripLabelKey(key); stripped == "" {

			slog.Error("FORWARDER: LOKI: LABEL FILTER: Label removed (illformed key)",
				slog.String("key", key))

			delete(labels, key)
			continue

		} else if stripped != key {

			slog.Warn("FORWARDER: LOKI: LABEL FILTER: Label moved (illformed key)",
				slog.String("key", key),
				slog.String("new_key", stripped))

			delete(labels, key)
			labels[stripped] = value
			key = stripped
		}

		if stripped := stripLabelValue(value); stripped == "" {

			slog.Error("FORWARDER: LOKI: LABEL FILTER: Label removed (stripped value is empty)",
				slog.String("key", key))

			delete(labels, key)

		} else if stripped != value {

			slog.Warn("FORWARDER: LOKI: LABEL FILTER: Label value stripped",
				slog.String("key", key))

			labels[key] = value
		}
	}
}

func stripLabelKey(key string) string {

	var stripped string

	for _, next := range key {

		switch next {
		case '_', '-':
			stripped += string(next)
			continue
		}

		if next >= '0' && next <= '9' {
			stripped += string(next)
			continue
		}

		if (next >= 'A' && next <= 'Z') || next >= 'a' && next <= 'z' {
			stripped += string(next)
			continue
		}
	}

	return stripped
}

func stripLabelValue(key string) string {

	var stripped string

	for _, next := range key {

		switch next {
		case '\\':
			stripped += "/"
			continue
		}

		if next >= 0x20 && next <= 0x7E {
			stripped += string(next)
			continue
		}

		stripped += "?"
	}

	return stripped
}

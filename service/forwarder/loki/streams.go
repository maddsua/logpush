package loki

import (
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/ingester/streams"
	"github.com/maddsua/logpush/service/logdata"
)

func webStreamToLabeled(stream *streams.WebStream, streamSource *dbops.Stream, txID uuid.UUID) []LokiStream {

	baseLabels := map[string]string{
		"logpush_source": "web",
		"service_name":   streamSource.Name,
		"logpush_tx":     txID.String(),
	}

	logdata.MergeStreamLabels(streamSource, baseLabels)
	logdata.CopyMetaFields(baseLabels, stream.Meta)

	var result []LokiStream

	for idx, entry := range stream.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		labels := map[string]string{}
		maps.Copy(labels, baseLabels)
		logdata.CopyMetaFields(labels, entry.Meta)
		labels["detected_level"] = entry.Level.String()

		result = append(result, LokiStream{
			Stream: labels,
			Values: [][]any{
				{
					entry.Date.String(idx),
					entry.Message,
				},
			},
		})
	}

	return result
}

func webStreamToStructured(stream *streams.WebStream, streamSource *dbops.Stream, txID uuid.UUID) LokiStream {

	labels := map[string]string{
		"logpush_source": "web",
		"service_name":   streamSource.Name,
		"logpush_tx":     txID.String(),
	}

	logdata.MergeStreamLabels(streamSource, labels)

	metaFields := map[string]string{}

	for key, val := range stream.Meta {

		if _, has := labels[key]; has {
			continue
		}

		switch key {
		case "env", "environment":
			labels["env"] = val
		default:
			metaFields[key] = val
		}
	}

	var streamValues [][]any
	for idx, entry := range stream.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		meta := map[string]string{}
		maps.Copy(meta, metaFields)
		meta["detected_level"] = entry.Level.String()

		if entry.Meta != nil {
			maps.Copy(meta, entry.Meta)
		}

		streamValues = append(streamValues, []any{
			entry.Date.String(idx),
			entry.Message,
			meta,
		})
	}

	return LokiStream{
		Stream: labels,
		Values: streamValues,
	}
}
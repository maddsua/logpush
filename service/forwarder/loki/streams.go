package loki

import (
	"maps"
	"strings"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
	"github.com/maddsua/logpush/service/ingester/streams"
	"github.com/maddsua/logpush/service/logdata"
)

func (this *Loki) webStreamToLabeled(logStream *streams.WebStream, instance *dbops.Stream, txID uuid.UUID) []LokiStream {

	baseLabels := map[string]string{
		"logpush_source": "web",
		"service_name":   instance.Name,
		"logpush_tx":     txID.String(),
	}

	logdata.MergeStreamLabels(baseLabels, instance)
	logdata.CopyMetaFields(baseLabels, logStream.Meta)

	var result []LokiStream

	for idx, entry := range logStream.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		labels := map[string]string{}
		maps.Copy(labels, baseLabels)
		logdata.CopyMetaFields(labels, entry.Meta)
		labels["level"] = entry.Level.String()

		if this.StrictLabels {
			filterLabelFormat(labels)
		}

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

func (this *Loki) webStreamToStructured(logStream *streams.WebStream, instance *dbops.Stream, txID uuid.UUID) LokiStream {

	labels := map[string]string{
		"logpush_source": "web",
		"service_name":   instance.Name,
		"logpush_tx":     txID.String(),
	}

	logdata.MergeStreamLabels(labels, instance)

	metaFields := map[string]string{}

	for key, val := range logStream.Meta {

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
	for idx, entry := range logStream.Entries {

		if entry.Message = strings.TrimSpace(entry.Message); entry.Message == "" {
			continue
		}

		meta := map[string]string{}
		maps.Copy(meta, metaFields)

		if entry.Meta != nil {
			maps.Copy(meta, entry.Meta)
		}

		meta["level"] = entry.Level.String()

		if this.StrictLabels {
			filterLabelFormat(meta)
		}

		streamValues = append(streamValues, []any{
			entry.Date.String(idx),
			entry.Message,
			meta,
		})
	}

	if this.StrictLabels {
		filterLabelFormat(labels)
	}

	return LokiStream{
		Stream: labels,
		Values: streamValues,
	}
}

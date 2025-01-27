package rest

import (
	"fmt"
	"strings"
)

func validateStaticLabels(labels map[string]string) error {

	if labels == nil {
		return nil
	}

	//	check that user doesn't dump a ton of static labels
	if len(labels) > MaxStaticLabels {
		return fmt.Errorf(`too many static labels (%d/%d)`, len(labels), MaxStaticLabels)
	}

	//	do some label validation to avoid exessive crap in your merged labels
	for key, value := range labels {

		var overwrite bool

		if trimmed := strings.TrimSpace(key); trimmed != key {
			delete(labels, key)
			key = trimmed
			overwrite = true
		}

		value = strings.TrimSpace(value)

		if !LabelNameExpr.MatchString(key) {
			return fmt.Errorf(`label key "%s" doesn't match the allowed format: '%s'`, key, LabelNameFormat)
		} else if len(value) > LabelValueMaxSize {
			return fmt.Errorf(`label's "%s" value is too long (%d/%d)`, key, len(value), LabelValueMaxSize)
		}

		if overwrite {
			labels[key] = value
		}
	}

	return nil
}

package logdata

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/maddsua/logpush/service/dbops"
)

type Level string

func (lvl Level) String() string {
	switch lvl {
	case "log", "warn", "error", "debug", "info":
		return string(lvl)
	default:
		return "error"
	}
}

type UnixMilli int64

func (um UnixMilli) String(sequence int) string {

	var ts time.Time
	if um > 0 {
		ts = time.UnixMilli(int64(um))
	} else {
		ts = time.Now()
	}

	return strconv.FormatInt(ts.UnixNano()+int64(sequence), 10)
}

func (um UnixMilli) Time(sequence int) time.Time {

	var ts time.Time
	if um > 0 {
		ts = time.UnixMilli(int64(um))
	} else {
		ts = time.Now()
	}

	return ts.Add(time.Duration(sequence))
}

func MergeStreamLabels(dst map[string]string, instance *dbops.Stream) map[string]string {

	if !instance.Labels.Valid {
		return dst
	}

	var streamLabels map[string]string
	if err := json.Unmarshal(instance.Labels.V, &streamLabels); err != nil {
		return dst
	}

	for key, val := range streamLabels {
		if mval, has := dst[key]; has {
			dst["_opt_"+key] = mval
		}
		dst[key] = val
	}

	return dst
}

func CopyMetaFields(dst map[string]string, src map[string]string) {

	if src == nil || dst == nil {
		return
	}

	for key, val := range src {
		if mval, has := dst[key]; has {
			dst["_entry_"+key] = mval
		}
		dst[key] = val
	}
}

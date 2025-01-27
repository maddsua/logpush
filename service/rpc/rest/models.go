package rest

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
)

const LabelNameFormat = `(?i)^[a-z_\-0-9]{1,50}$`
const LabelValueMaxSize = 250
const MaxStaticLabels = 25

const AppNameFormat = `^[\w\d \-\_]{3,50}$`

var LabelNameExpr = regexp.MustCompile(LabelNameFormat)
var AppNameExpr = regexp.MustCompile(AppNameFormat)

type Date struct {
	time.Time
}

func (this *Date) UnmarshalJSON(data []byte) error {

	ts, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}

	this.Time = time.Unix(ts, 0)
	return nil
}

func (this Date) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(this.Unix(), 10)), nil
}

type Stream struct {
	ID        uuid.UUID         `json:"id"`
	CreatedAt Date              `json:"created_at"`
	Name      string            `json:"name"`
	Labels    map[string]string `json:"labels"`
}

func (this *Stream) ScanRow(row dbops.Stream) error {

	this.ID = row.ID
	this.CreatedAt.Time = row.CreatedAt
	this.Name = row.Name

	if row.Labels.Valid {
		if err := json.Unmarshal(row.Labels.V, &this.Labels); err != nil {
			return err
		}
	} else {
		this.Labels = map[string]string{}
	}

	return nil
}

type StreamListItem struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt Date      `json:"created_at"`
	Name      string    `json:"name"`
}

func (this *StreamListItem) ScanRow(row dbops.ListStreamsRow) error {

	this.ID = row.ID
	this.CreatedAt.Time = row.CreatedAt
	this.Name = row.Name

	return nil
}

type AddStreamParams struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels"`
}

func (this *AddStreamParams) ReadBody(reader io.Reader) error {

	if err := json.NewDecoder(reader).Decode(this); err != nil {
		return err
	}

	if this.Name = strings.TrimSpace(this.Name); this.Name == "" {
		return errors.New("invalid stream name format")
	}

	return nil
}

func (this AddStreamParams) Row() dbops.AddStreamParams {

	var labels sql.Null[[]byte]
	if len(this.Labels) > 0 {
		labels.V, _ = json.Marshal(this.Labels)
		labels.Valid = true
	}

	return dbops.AddStreamParams{
		Name:   this.Name,
		Labels: labels,
	}
}

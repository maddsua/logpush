package rest

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/maddsua/logpush/service/dbops"
)

type RPCProcedures struct {
	DB *dbops.Queries
}

func (this *RPCProcedures) StreamsList(writer http.ResponseWriter, req *http.Request) {

	entries, err := this.DB.ListStreams(req.Context())
	if err != nil {
		writeJsonError(writer, err, http.StatusInternalServerError)
		return
	}

	result := make([]StreamListItem, len(entries))
	for idx, entry := range entries {
		if err := result[idx].ScanRow(entry); err != nil {
			writeJsonError(writer, err, http.StatusInternalServerError)
			return
		}
	}

	writeJsonData(writer, result)
}

func (this *RPCProcedures) StreamsGet(writer http.ResponseWriter, req *http.Request) {

	id, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	entry, err := this.DB.GetStream(req.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJsonError(writer, errors.New("stream doesn't exist"), http.StatusNotFound)
		} else {
			writeJsonError(writer, err, http.StatusInternalServerError)
		}
		return
	}

	var result Stream
	if err := result.ScanRow(entry); err != nil {
		writeJsonError(writer, err, http.StatusInternalServerError)
		return
	}

	writeJsonData(writer, result)
}

func (this *RPCProcedures) StreamsDelete(writer http.ResponseWriter, req *http.Request) {

	id, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	if affected, err := this.DB.DeleteStream(req.Context(), id); err != nil {
		writeJsonError(writer, err, http.StatusInternalServerError)
		return
	} else if affected == 0 {
		writeJsonError(writer, errors.New("stream doesn't exist"), http.StatusNotFound)
		return
	}

	slog.Info("RPC: Removed stream",
		slog.String("remote_addr", req.RemoteAddr),
		slog.String("stream_id", id.String()))

	writer.WriteHeader(http.StatusOK)
}

func (this *RPCProcedures) StreamsAdd(writer http.ResponseWriter, req *http.Request) {

	if err := assetJSON(req); err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	var params AddStreamParams
	if err := params.ReadBody(req.Body); err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	entry, err := this.DB.AddStream(req.Context(), params.Row())
	if err != nil {
		//	todo: handle id/name collision
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	var result Stream
	if err := result.ScanRow(entry); err != nil {
		writeJsonError(writer, err, http.StatusInternalServerError)
		return
	}

	slog.Info("RPC: Added stream",
		slog.String("remote_addr", req.RemoteAddr),
		slog.String("stream_id", entry.ID.String()),
		slog.String("stream_name", entry.Name))

	writeJsonData(writer, result)
}

func (this *RPCProcedures) StreamsSetLabels(writer http.ResponseWriter, req *http.Request) {

	id, err := uuid.Parse(req.PathValue("id"))
	if err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	if err := assetJSON(req); err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	var payload map[string]string
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
		writeJsonError(writer, err, http.StatusBadRequest)
		return
	}

	var labels sql.Null[[]byte]
	if len(payload) > 0 {
		data, _ := json.Marshal(payload)
		labels.V = data
		labels.Valid = true
	}

	entry, err := this.DB.SetStreamLabels(req.Context(), dbops.SetStreamLabelsParams{ID: id, Labels: labels})
	if err != nil {
		if err == sql.ErrNoRows {
			writeJsonError(writer, errors.New("stream doesn't exist"), http.StatusNotFound)
		} else {
			writeJsonError(writer, err, http.StatusInternalServerError)
		}
		return
	}

	slog.Info("RPC: Updated stream labels",
		slog.String("remote_addr", req.RemoteAddr),
		slog.String("stream_id", id.String()))

	var result Stream
	if err := result.ScanRow(entry); err != nil {
		writeJsonError(writer, err, http.StatusInternalServerError)
		return
	}

	writeJsonData(writer, result)
}

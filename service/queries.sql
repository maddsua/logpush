-- name: GetStream :one
select * from streams where id = sqlc.arg(id);

-- name: InsertStreamEntry :exec
insert into stream_entries (
	stream_id,
	created_at,
	level,
	message,
	metadata
) values (
	sqlc.arg(stream_id),
	sqlc.arg(created_at),
	sqlc.arg(level),
	sqlc.arg(message),
	sqlc.narg(metadata)
);

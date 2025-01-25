-- name: GetStream :one
select * from streams where id = sqlc.arg(id);

-- name: ListStreams :many
select id, created_at, name from streams;

-- name: DeleteStream :execrows
delete from streams where id = sqlc.arg(id);

-- name: AddStream :one
insert into streams (
	name,
	labels
) values (
	sqlc.arg(name),
	sqlc.narg(labels)
) returning *;

-- name: InsertStreamEntry :exec
insert into stream_entries (
	stream_id,
	tx_id,
	created_at,
	level,
	message,
	metadata
) values (
	sqlc.arg(stream_id),
	sqlc.narg(tx_id),
	sqlc.arg(created_at),
	sqlc.arg(level),
	sqlc.arg(message),
	sqlc.narg(metadata)
);

-- name: SetStreamLabels :one
update streams
set labels = sqlc.narg(labels)
where id = sqlc.arg(id)
returning *;

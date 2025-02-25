-- name: InsertEntry :exec
insert into entries (
	time,
	level,
	message,
	labels,
	meta,
	tx_id,
	stream_tag
) values (
	sqlc.arg(time),
	sqlc.arg(level),
	sqlc.arg(message),
	sqlc.arg(labels),
	sqlc.arg(meta),
	sqlc.arg(tx_id),
	sqlc.arg(stream_tag)
);

-- name: GetEntriesRange :many
select * from entries
where time >= sqlc.arg(range_from)
	and time <= sqlc.arg(range_to);

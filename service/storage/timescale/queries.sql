-- name: InsertEntry :exec
insert into entries (
	time,
	level,
	message,
	labels,
	meta,
	tx_id,
	service_name
) values (
	sqlc.arg(time),
	sqlc.arg(level),
	sqlc.arg(message),
	sqlc.arg(labels),
	sqlc.arg(meta),
	sqlc.arg(tx_id),
	sqlc.arg(service_name)
);

-- name: GetEntriesRange :many
select * from entries
where time >= sqlc.arg(range_from)
	and time <= sqlc.arg(range_to);

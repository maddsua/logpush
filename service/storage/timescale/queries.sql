-- name: InsertEntry :exec
insert into entries (
	time,
	level,
	message,
	labels,
	metadata
) values (
	sqlc.arg(time),
	sqlc.arg(level),
	sqlc.arg(message),
	sqlc.arg(labels),
	sqlc.arg(metadata)
);

-- name: GetEntriesRange :many
select * from entries
where time >= sqlc.arg(range_from)
	and time <= sqlc.arg(range_to);

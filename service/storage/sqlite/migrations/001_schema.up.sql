create table entries (
	id integer primary key autoincrement,
	time integer not null,
	stream_tag text not null,
	level text not null,
	message text not null,
	labels blob,
	meta blob,
	tx_id text
);

create table entries (
	id integer primary key autoincrement,
	time integer not null,
	level text not null,
	message text not null,
	labels blob,
	meta blob,
	tx_id text,
	service_name text
);

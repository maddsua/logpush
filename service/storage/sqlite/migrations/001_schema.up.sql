create table entries (
	id integer primary key autoincrement,
	time integer not null,
	level text not null,
	message text not null,
	labels blob null,
	meta blob null,
	tx_id text null,
	service_name text null
);

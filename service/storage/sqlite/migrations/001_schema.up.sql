create table entries (
	id integer primary key autoincrement,
	time integer not null,
	level text not null,
	message text not null,
	labels blob null,
	metadata blob null
);

create table streams (
	id uuid not null default gen_random_uuid () primary key,
	created_at timestamp with time zone not null default now(),
	name text not null unique,
	labels jsonb null
);

create table stream_entries (
	id uuid not null default gen_random_uuid () primary key,
	created_at timestamp with time zone not null default now(),
	stream_id uuid not null references streams(id) on update cascade on delete cascade,
	level text not null,
	message text not null,
	metadata jsonb null
);

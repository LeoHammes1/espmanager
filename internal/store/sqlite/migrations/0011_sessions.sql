create table sessions (
  id text primary key,
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  expires_at text not null
) strict;

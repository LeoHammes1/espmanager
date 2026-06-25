create table claim_tokens (
  token text primary key,
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  expires_at text not null
) strict;

create table device_credentials (
  device_id text primary key,
  password_hash text not null,
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ'))
) strict;

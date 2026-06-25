create table drivers (
  id text primary key,
  name text not null,
  repo_url text not null,
  branch text not null default 'main',
  pio_env text not null default '',
  webhook_secret text not null default '',
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ'))
) strict;

create index drivers_repo_url_idx on drivers (repo_url);

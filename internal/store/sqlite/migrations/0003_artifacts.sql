create table firmware_artifacts (
  driver_id text not null,
  version text not null,
  commit_sha text not null default '',
  env text not null default '',
  sha256 text not null,
  signature text not null,
  size integer not null default 0,
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  primary key (driver_id, version)
) strict;

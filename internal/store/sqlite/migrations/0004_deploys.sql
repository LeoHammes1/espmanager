create table deploys (
  id text primary key,
  driver_id text not null,
  version text not null,
  created_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ'))
) strict;

create table deploy_targets (
  deploy_id text not null,
  device_id text not null,
  version text not null,
  status text not null,
  updated_at text not null default (strftime('%Y-%m-%dT%H:%M:%fZ')),
  primary key (deploy_id, device_id)
) strict;

create index deploy_targets_device_idx on deploy_targets (device_id, updated_at desc);

create table artifact_sequence (
  id integer primary key check (id = 1),
  value integer not null
) strict;

insert into artifact_sequence (id, value)
  select 1, coalesce(max(sequence), 0) from firmware_artifacts;

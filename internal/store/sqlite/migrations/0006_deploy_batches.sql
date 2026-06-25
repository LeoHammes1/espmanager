alter table deploys add column state text not null default 'in_progress';

alter table deploy_targets add column batch integer not null default 0;

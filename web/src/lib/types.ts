// Wire types for the ESPManager JSON API. camelCase throughout.

export interface SessionState {
  authenticated: boolean;
  setupRequired: boolean;
  user: string;
}

export interface ErrorResponse {
  error: string;
  message?: string;
}

// ---- Devices ----
export interface Device {
  id: string;
  name: string;
  reportedVersion: string;
  lastSeenAt: string | null; // RFC3339; null = never seen
  driverId: string;
  driverName: string;
  online: boolean;
}

export interface DriverOption {
  id: string;
  name: string;
}

export interface DevicesResponse {
  devices: Device[];
  drivers: DriverOption[];
}

export interface EnrollResponse {
  token: string;
  expiresAt: string;
}

export interface RotateResponse {
  deviceId: string;
  password: string;
  delivered: boolean;
}

// ---- Drivers ----
export interface Driver {
  id: string;
  name: string;
  repoUrl: string;
  branch: string;
  pioEnv: string;
  webhookPath: string;
  webhookUrl: string;
  createdAt: string;
}

export interface ListDriversResponse {
  drivers: Driver[];
}

export interface CreateDriverRequest {
  name: string;
  repoUrl: string;
  branch?: string;
  pioEnv?: string;
}

export interface CreateDriverResponse {
  driver: Driver;
  webhookSecret: string;
}

// ---- Deploys + Overview ----
export type DeployState = "in_progress" | "paused" | "completed" | "cancelled";
export type TargetStatus =
  | "pending"
  | "triggered"
  | "downloading"
  | "succeeded"
  | "failed"
  | "lost";

export interface DeployCounts {
  total: number;
  succeeded: number;
  inflight: number;
  failed: number;
  lost: number;
  pending: number;
  atRisk: boolean;
  succeededPct: number;
  inflightPct: number;
  failedPct: number;
  lostPct: number;
}

export interface DeployRow {
  id: string;
  driver: string;
  version: string;
  state: DeployState;
  stateText: string;
  counts: DeployCounts;
  createdAt: string;
}

export interface TargetRow {
  deviceId: string;
  deviceName: string;
  sequence: string;
  batch: number;
  status: TargetStatus;
  updatedAt: string;
}

export interface BatchView {
  batch: number;
  label: string;
  counts: DeployCounts;
  targets: TargetRow[];
}

export interface PauseInfo {
  batchLabel: string;
  failed: number;
  lost: number;
  total: number;
  threshold: number;
}

export interface DeployDetail {
  id: string;
  driver: string;
  version: string;
  state: DeployState;
  stateText: string;
  createdAt: string;
  counts: DeployCounts;
  batches: BatchView[];
  pause: PauseInfo | null;
}

export interface DeviceRef {
  id: string;
  name: string;
  lastSeenAt: string | null;
}

export interface FailedRef {
  deployId: string;
  deviceId: string;
  deviceName: string;
  driver: string;
  version: string;
  status: TargetStatus;
}

export interface OverviewResponse {
  devicesOnline: number;
  devicesTotal: number;
  attentionCount: number;
  rollouts: DeployRow[];
  offline: DeviceRef[];
  failedUpdates: FailedRef[];
}

export type LiveStatus = "connecting" | "open";

export interface DriveInfo {
  drive: string;
  total: number;
  free: number;
  usedPct: number;
  status: 'ok' | 'warn' | 'critical';
}

export interface DirUsage {
  path: string;
  name: string;
  bytes: number;
  fileCount: number;
  category: string;
  safety: 'safe' | 'caution' | 'protected';
  depth: number;
}

export interface ScanReport {
  rootPath: string;
  scannedBytes: number;
  scannedFiles: number;
  durationMs: number;
  topDirectories: DirUsage[];
  cleanable: DirUsage[];
}

export interface CleanResult {
  paths: string[];
  dryRun: boolean;
  freedBytes: number;
  deletedFiles: number;
  skippedBusy: number;
  errors: string[];
  recordId: string;
}

export interface ProcessReport {
  name: string;
  pid: string;
  path: string;
  blocking: boolean;
}

export interface GuardConfirmation {
  optInOverride: boolean;
  confirmedProcesses: string[];
  confirmationToken: string;
}

export interface GuardDecision {
  path: string;
  allowed: boolean;
  requiresConfirmation: boolean;
  blockingProcesses: ProcessReport[];
  reason?: string;
}

export type EmpireDomain = 'antigravity' | 'docker' | 'go' | 'cargo' | 'rocksoul';

export interface EmpireCacheGroup {
  domain: EmpireDomain;
  label: string;
  safety: 'safe' | 'caution' | 'protected';
  reclaimBytes: number;
  fileCount: number;
  paths: string[];
  presentTargets: number;
}

export interface EmpireSweepSummary {
  groups: EmpireCacheGroup[];
  totalReclaimBytes: number;
  safeReclaimBytes: number;
  generatedAt: string;
}

export interface TrayWatchdogAlert {
  drive: string;
  level: 'ok' | 'warn' | 'critical';
  freeBytes: number;
  usedPct: number;
  tooltip: string;
  requiresConfirmation: boolean;
  recommendEmpireSweep: boolean;
  checkedAt: string;
}

export interface HistoryRecord {
  when: string;
  kind: string;
  root: string;
  paths: string[];
  dryRun: boolean;
  freed: number;
  deleted: number;
  skipped: number;
  recordId: string;
}

declare global {
  interface Window {
    go?: {
      main?: { App?: AppBindings };
    };
    main?: { App?: AppBindings };
  }
}

export interface AppBindings {
  ListDrives: () => Promise<DriveInfo[]>;
  ScanPath: (path: string, maxDepth: number) => Promise<ScanReport>;
  QuickAudit: () => Promise<DirUsage[]>;
  EmpireSweep?: () => Promise<EmpireSweepSummary>;
  TrayWatchdog?: (autoSweepOptIn: boolean) => Promise<TrayWatchdogAlert[]>;
  GuardPath: (path: string) => Promise<ProcessReport[]>;
  ConfirmGuard?: (path: string, confirmation: GuardConfirmation) => Promise<GuardDecision>;
  DryRun: (paths: string[]) => Promise<CleanResult>;
  Clean: (paths: string[]) => Promise<CleanResult>;
  CleanWithConfirmation?: (paths: string[], confirmation: GuardConfirmation | null) => Promise<CleanResult>;
  History: (limit: number) => Promise<HistoryRecord[]>;
  LabNotesPath: () => Promise<string>;
  OpenCleanMgr: () => Promise<void>;
  HomeDir: () => Promise<string>;
}

// Wails exposes bindings at window.go.main.App; some builds use window.main.App.
export const api = {
  get bindings(): AppBindings | undefined {
    return window.go?.main?.App ?? window.main?.App;
  },
};

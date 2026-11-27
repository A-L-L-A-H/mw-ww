import { useCallback, useEffect, useState } from 'react';
import { Button } from '@moonwitness/ui/components/button';
import { Checkbox } from '@moonwitness/ui/components/checkbox';
import { Badge } from '@moonwitness/ui/components/badge';
import { ConfirmDialog } from '@moonwitness/ui/components/confirm-dialog';
import { Input } from '@moonwitness/ui/components/input';
import { MetricTile } from '@moonwitness/ui/components/metric-tile';
import { Panel } from '@moonwitness/ui/components/panel';
import { api, type CleanResult, type DirUsage, type DriveInfo, type HistoryRecord, type ProcessReport, type ScanReport } from './api';
import './styles.css';

// Wails injects bindings asynchronously; read through the getter at call time.
const B = () => api.bindings;

function gb(bytes: number) {
  return `${(bytes / 1024 ** 3).toFixed(2)} GB`;
}

function SafetyBadge({ safety }: { safety: DirUsage['safety'] }) {
  const label = safety === 'safe' ? 'SAFE' : safety === 'caution' ? 'CAUTION' : 'LOCKED';
  return <Badge variant={safety === 'safe' ? 'cyan' : safety === 'caution' ? 'magenta' : 'destructive'}>{label}</Badge>;
}

export function App() {
  const [drives, setDrives] = useState<DriveInfo[]>([]);
  const [path, setPath] = useState('');
  const [report, setReport] = useState<ScanReport | null>(null);
  const [targets, setTargets] = useState<DirUsage[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [history, setHistory] = useState<HistoryRecord[]>([]);
  const [notesPath, setNotesPath] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [preview, setPreview] = useState<CleanResult | null>(null);
  const [guards, setGuards] = useState<ProcessReport[]>([]);

  const refresh = useCallback(async () => {
    if (!B()!.ListDrives) return; // Wails bindings not ready yet
    try {
      setDrives(await B()!.ListDrives());
      setHistory(await B()!.History(30));
      setNotesPath(await B()!.LabNotesPath());
      const home = await B()!.HomeDir();
      setPath((current) => current || home);
      setTargets(await B()!.QuickAudit());
    } catch (err) {
      setError(String(err));
    }
  }, []);

  useEffect(() => {
    // Wails injects window.go.main.App asynchronously; poll briefly until ready.
    let attempts = 0;
    const timer = window.setInterval(() => {
      if (B()) {
        window.clearInterval(timer);
        void refresh();
      } else if (++attempts > 40) {
        window.clearInterval(timer);
        setError('Wails bindings tidak tersedia. Restart mw-ww.exe.');
      }
    }, 100);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const scan = async () => {
    if (!path.trim() || !B()!.ScanPath) return;
    setBusy('scan'); setError('');
    try {
      setReport(await B()!.ScanPath(path.trim(), 4));
      setHistory(await B()!.History(30));
    } catch (err) {
      setError(String(err));
    } finally { setBusy(null); }
  };

  const toggle = async (target: DirUsage) => {
    // Process guard: never select a target owned by a running app.
    if (!selected.has(target.path)) {
      const hits = await B()!.GuardPath(target.path);
      setGuards(hits);
      if (hits.some((h) => h.blocking)) {
        setError(`Aplikasi masih jalan: ${hits.map((h) => h.name).join(', ')}. Tutup dulu sebelum memilih.`);
        return;
      }
    }
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(target.path)) next.delete(target.path); else next.add(target.path);
      return next;
    });
  };

  const doDryRun = async () => {
    setBusy('dry'); setError('');
    try {
      setPreview(await B()!.DryRun([...selected]));
      setConfirmOpen(true);
    } catch (err) { setError(String(err)); }
    finally { setBusy(null); }
  };

  const doClean = async () => {
    setBusy('clean');
    try {
      const result = await B()!.Clean([...selected]);
      setConfirmOpen(false);
      setSelected(new Set());
      setPreview(null);
      await refresh();
      if (report) setReport(await B()!.ScanPath(report.rootPath, 4));
      setError(result.errors.join('\n'));
    } catch (err) { setError(String(err)); }
    finally { setBusy(null); }
  };

  const rows = targets.length > 0 ? targets : (report?.cleanable ?? []);
  const cDrive = drives.find((d) => d.drive === 'C');
  const selectedBytes = rows.filter((r) => selected.has(r.path)).reduce((sum, r) => sum + r.bytes, 0);

  return (
    <main className="ww-shell">
      <header className="ww-masthead">
        <h1 className="ww-title">mw-ww</h1>
        <span className="ww-tagline">I AM THE ONE WHO SWEEPS ∗ clean code · clean disk · clean conscience</span>
      </header>

      {error && <p className="ww-error" role="alert">{error}</p>}

      <div className="ww-grid">
        <MetricTile label="C: FREE" value={cDrive ? gb(cDrive.free) : '—'} detail={cDrive ? `${cDrive.usedPct}% used` : ''}
          accent={cDrive?.status === 'critical' ? 'magenta' : 'cyan'} surface="ink" frame="scuffed" size="compact" />
        <MetricTile label="SWEEP TARGETS" value={String(rows.length)} detail="safe & caution folders" accent="yellow" surface="ink" frame="scuffed" size="compact" />
        <MetricTile label="SELECTED" value={gb(selectedBytes)} detail={`${selected.size} folder(s)`} accent="violet" surface="ink" frame="scuffed" size="compact" />
      </div>

      <Panel title="THE LAB — start a sweep" description="Scan a root path, or use the quick cache audit." surface="ink" frame="scuffed" density="compact">
        <div className="ww-toolbar">
          <label className="ww-path" htmlFor="ww-path">Root path</label>
          <Input id="ww-path" value={path} onChange={(e) => setPath(e.target.value)} className="ww-path" />
          <Button onClick={scan} disabled={busy === 'scan'}>{busy === 'scan' ? 'Sweeping…' : 'Scan'}</Button>
          <Button variant="secondary" onClick={() => void refresh()}>Refresh</Button>
          <Button variant="secondary" onClick={() => void B()!.OpenCleanMgr()}>Windows Cleanup</Button>
        </div>
        {report && <p className="ww-tagline">Scanned {gb(report.scannedBytes)} · {report.scannedFiles} files · {report.durationMs} ms</p>}
      </Panel>

      <Panel title="EVIDENCE BOARD" description="Tick a folder to prepare the sweep. Locked rows cannot be touched." surface="ink" frame="line" density="compact" className="ww-notes">
        <table className="ww-table">
          <thead><tr><th aria-label="Select" /><th>Category</th><th>Safety</th><th>Size</th><th>Path</th></tr></thead>
          <tbody>
            {rows.map((row) => {
              const locked = row.safety === 'protected';
              return (
                <tr key={row.path} className={selected.has(row.path) ? 'ww-selected' : ''}>
                  <td>
                    <Checkbox
                      aria-label={`Select ${row.path}`}
                      checked={selected.has(row.path)}
                      disabled={locked}
                      onChange={() => void toggle(row)}
                    />
                  </td>
                  <td>{row.category}</td>
                  <td><SafetyBadge safety={row.safety} /></td>
                  <td>{gb(row.bytes)}</td>
                  <td className="ww-path-cell" title={row.path}>{row.path}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <div className="ww-toolbar">
          <Button disabled={selected.size === 0 || busy !== null} onClick={doDryRun}>
            {busy === 'dry' ? 'Checking…' : 'Dry run & confirm'}
          </Button>
        </div>
      </Panel>

      {guards.length > 0 && (
        <div className="ww-alert" role="status">
          Process guard: {guards.map((g) => `${g.name}${g.pid ? ` (PID ${g.pid})` : ''}`).join(', ')} owns this path.
        </div>
      )}

      <Panel title="LAB NOTES" description={notesPath} surface="ink" frame="line" density="compact" className="ww-notes">
        <table className="ww-table">
          <thead><tr><th>When</th><th>Kind</th><th>Freed</th><th>Files</th><th>Target</th></tr></thead>
          <tbody>
            {history.map((h) => (
              <tr key={`${h.recordId}-${h.when}`}>
                <td>{h.when}</td><td>{h.kind}{h.dryRun ? ' (dry)' : ''}</td>
                <td>{gb(h.freed)}</td><td>{h.deleted}</td>
                <td className="ww-path-cell" title={h.root || h.paths.join('; ')}>{h.root || h.paths.join('; ')}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </Panel>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Say my name — confirm the sweep"
        description={`About to clean ${selected.size} folder(s).`}
        warningBanner={preview?.errors.join('\n')}
        consequence={`Recoverable estimate: ${gb(preview?.freedBytes ?? 0)} across ${preview?.deletedFiles ?? 0} files. Locked files will be skipped.`}
        confirmLabel="Sweep it"
        cancelLabel="Not yet"
        destructive
        busy={busy === 'clean'}
        onConfirm={doClean}
      />
    </main>
  );
}

#!/usr/bin/env python3
"""Measure an explicitly supplied standalone command and owned Ollama PID.

Uses only Python's standard library and macOS read-only process/VM interfaces.
Process RSS and system-wide swap are separate observations, not GPU accounting.
"""
import argparse
import datetime
import json
import subprocess
import time
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument('--server-pid', required=True, type=int)
p.add_argument('--output', required=True, type=Path)
p.add_argument('command', nargs=argparse.REMAINDER)
a = p.parse_args()
command = a.command[1:] if a.command[:1] == ['--'] else a.command
if not command or a.output.exists():
    p.error('a command and a new output path are required')

def sample(child_pid):
    rows = subprocess.check_output(['/bin/ps', '-axo', 'pid,ppid,rss'], text=True)
    processes = [tuple(map(int, row.split())) for row in rows.splitlines()[1:]]
    def family(seed):
        ids = {seed}
        for _ in range(8):
            new = {pid for pid, parent, _ in processes if parent in ids}
            if new <= ids:
                break
            ids |= new
        return [(pid, rss) for pid, _, rss in processes if pid in ids]
    server = family(a.server_pid)
    runner = family(child_pid) if child_pid else []
    return {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
            'server_processes_rss_kib': server, 'runner_processes_rss_kib': runner,
            'server_rss_kib_sum': sum(rss for _, rss in server),
            'runner_rss_kib_sum': sum(rss for _, rss in runner),
            'host_swap': subprocess.check_output(['/usr/sbin/sysctl', 'vm.swapusage'], text=True).strip()}

baseline = sample(None)
start = time.monotonic()
proc = subprocess.Popen(command)
samples = []
try:
    while proc.poll() is None:
        samples.append(sample(proc.pid))
        if len(samples) >= 360:
            proc.terminate()
            raise RuntimeError('measurement reached the 30-minute bound; command terminated')
        time.sleep(5)
finally:
    if proc.poll() is None:
        proc.terminate()
    try:
        code = proc.wait(timeout=5)
    except subprocess.TimeoutExpired:
        proc.kill()
        code = proc.wait()
    result = {'schema_version': 1, 'command': command, 'owned_server_pid': a.server_pid,
              'sampling_seconds': 5, 'baseline': baseline, 'samples': samples,
              'final': sample(None), 'elapsed_seconds': time.monotonic()-start,
              'exit_code': code, 'limitations': 'RSS excludes a reliable account of Metal/unified GPU allocations; host swap is system-wide and cannot be attributed to this run. Runner RSS is the standalone Go process, not a production Evie foreground workload.'}
    with a.output.open('x') as f:
        json.dump(result, f, indent=2)
        f.write('\n')
raise SystemExit(code)

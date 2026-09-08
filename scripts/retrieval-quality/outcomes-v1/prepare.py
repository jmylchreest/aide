#!/usr/bin/env python3
"""Recreate isolated candidates from pinned Git source and frozen templates."""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys

package = Path(__file__).resolve().parent
repo = package.parents[2]
protocol = json.loads((package / 'protocol.json').read_text())
pins = json.loads((package / 'navigation/sources.json').read_text())
destination = Path(sys.argv[1]).resolve()
destination.mkdir(parents=True, exist_ok=False)
for trial in protocol['trial_order']:
    root = destination / trial['id']
    if trial['task'] == 'navigation':
        for name, digest in pins['files'].items():
            data = subprocess.check_output(['git', 'show', f"{pins['commit']}:{name}"], cwd=repo)
            if hashlib.sha256(data).hexdigest() != digest:
                raise SystemExit(f'Pinned source mismatch: {name}')
            target = root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(data)
    else:
        shutil.copytree(package / trial['task'] / 'template', root)
    print(root)

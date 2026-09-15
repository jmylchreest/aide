#!/usr/bin/env python3
"""Offline functional grader. Never put this file in a participant root."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

parser = argparse.ArgumentParser()
parser.add_argument('task', choices=['debug', 'edit'])
parser.add_argument('root', type=Path)
args = parser.parse_args()
with tempfile.TemporaryDirectory(prefix='aide-outcome-grade-') as tmp:
    report = Path(tmp) / 'junit.xml'
    command = ['rtk', 'proxy', 'bun', 'test', '--reporter=junit', '--reporter-outfile', str(report), str(Path(__file__).parent / args.task / 'grader/hidden.test.ts')]
    run = subprocess.run(command, env={**os.environ, 'AIDE_TRIAL_ROOT': str(args.root.resolve())}, capture_output=True, text=True)
    checks = []
    if report.exists():
        for case in ET.parse(report).iter('testcase'):
            checks.append({'name': case.get('name'), 'pass': case.find('failure') is None and case.find('error') is None and case.find('skipped') is None})
    result = {'task': args.task, 'exit_code': run.returncode, 'checks': checks, 'passed': bool(checks) and run.returncode == 0 and all(c['pass'] for c in checks), 'stdout': run.stdout, 'stderr': run.stderr}
    print(json.dumps(result, indent=2))
    raise SystemExit(0 if result['passed'] else 1)

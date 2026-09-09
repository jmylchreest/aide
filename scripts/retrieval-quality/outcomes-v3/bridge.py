#!/usr/bin/env python3
"""Benchmark-only, fixture-bound transport to the real aide stdio MCP handlers.

The index is seeded during controller setup and is NOT refreshed by calls.
Search/references can therefore be stale after participant edits. Source tools
retain aide's current-source behavior. Every call pays and records MCP startup,
identity verification, request, and shutdown costs. This is not a filesystem
sandbox: participant shell scope and changes to .aide still require trace audit.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import selectors
import stat
import subprocess
import sys
import time
import uuid


MARKER = "retrieval-bridge.json"
TOOLS = {
    "code_search": {"query", "kind", "lang", "file", "limit"},
    "code_references": {"symbol", "symbols", "kind", "file", "limit"},
    "code_symbols": {"file"},
    "code_outline": {"file", "keep_comments"},
    "code_read_symbol": {"symbol", "symbols", "kind", "file", "start_line"},
}
VCS = {".git", ".hg", ".svn", ".bzr", ".fossil"}


def strict_json(text):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise ValueError("Duplicate JSON key")
            result[key] = value
        return result

    def invalid(value):
        raise ValueError(f"Invalid JSON constant: {value}")

    return json.loads(text, object_pairs_hook=pairs, parse_constant=invalid)


def absolute_path(value):
    path = Path(value)
    if not path.is_absolute() or ".." in path.parts:
        raise ValueError("Paths must be absolute and contain no traversal")
    for part in (path, *path.parents):
        if part.is_symlink():
            raise ValueError("Symlinks are not allowed in configured paths")
    return path.resolve()


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def source_inventory(root):
    """Reject symlinks/special files without descending into runtime .aide data."""
    files = {}
    for directory, dirs, names in os.walk(root, followlinks=False):
        directory = Path(directory)
        for name in [*dirs, *names]:
            path = directory / name
            if path.is_symlink():
                raise ValueError("Fixture symlinks are not allowed")
            if name in VCS:
                raise ValueError("Fixture must not contain VCS markers")
            if name == ".aide":
                if directory != root:
                    raise ValueError("Nested aide roots are not allowed")
                continue
            mode = path.stat().st_mode
            if stat.S_ISREG(mode):
                files[path.relative_to(root).as_posix()] = sha256(path)
            elif not stat.S_ISDIR(mode):
                raise ValueError("Fixture special files are not allowed")
        if directory == root and ".aide" in dirs:
            dirs.remove(".aide")
    return files


def configuration(root, binary, evidence_dir):
    root, binary, evidence_dir = map(absolute_path, (root, binary, evidence_dir))
    if not root.is_dir() or not binary.is_file() or not os.access(binary, os.X_OK):
        raise ValueError("Existing fixture directory and executable binary are required")
    if evidence_dir == root or root in evidence_dir.parents or evidence_dir in root.parents:
        raise ValueError("Evidence must be outside the fixture and not its ancestor")
    if binary == root or root in binary.parents:
        raise ValueError("The binary must be outside the participant fixture")
    return root, binary, evidence_dir


def child_environment(root):
    # No inherited HOME, AIDE_*, config, credentials, or live socket settings.
    return {
        "PATH": "/usr/bin:/bin", "AIDE_PROJECT_ROOT": str(root),
        "AIDE_CODE_STORE_ENABLED": "true", "AIDE_CODE_STORE_SYNC": "true",
        "AIDE_CODE_WATCH": "false", "AIDE_CODE_WATCH_PATHS": "",
        "AIDE_INDEX_NON_VCS": "false", "AIDE_GRAMMAR_AUTO_DOWNLOAD": "0",
        "AIDE_CLEANUP_ENABLED": "false", "AIDE_MAINTENANCE_COMPACT_ON_EXIT": "false",
        "AIDE_PPROF_ENABLE": "false",
    }


def write_json(path, value):
    with path.open("x", encoding="utf-8") as stream:
        json.dump(value, stream, indent=2, ensure_ascii=False, allow_nan=False)
        stream.write("\n")


def prepare(root, binary, evidence_dir, timeout=30):
    """Controller-only initialization; never invoke against an existing aide root."""
    started = time.monotonic()
    root, binary, evidence_dir = configuration(root, binary, evidence_dir)
    if (root / ".aide").exists():
        raise ValueError("Setup requires a pristine fixture without .aide")
    inventory = source_inventory(root)
    if not inventory:
        raise ValueError("Empty fixtures are not allowed")
    evidence_dir.mkdir(parents=True, exist_ok=True)
    if any(evidence_dir.iterdir()):
        raise ValueError("Setup requires an empty evidence directory")
    binding = {
        "schema_version": 1, "id": uuid.uuid4().hex, "root": str(root),
        "binary": str(binary), "binary_sha256": sha256(binary),
        "evidence_dir": str(evidence_dir), "seed_files": inventory,
        "index_policy": "seeded; no automatic refresh after participant edits",
    }
    (root / ".aide").mkdir()
    record = {"operation": "prepare", "binding": binding, "error": None}
    index_started = time.monotonic()
    try:
        result = subprocess.run(
            [str(binary), "code", "index", str(root), "--force"], cwd=root,
            env=child_environment(root), capture_output=True, timeout=timeout,
        )
        record.update(returncode=result.returncode,
                      stdout=result.stdout.decode("utf-8", errors="replace"),
                      stderr=result.stderr.decode("utf-8", errors="replace"))
        if result.returncode:
            raise RuntimeError("Fixture index setup failed; inspect controller evidence")
        if source_inventory(root) != inventory:
            raise RuntimeError("Fixture source changed during setup")
        write_json(root / ".aide" / MARKER, binding)
        return binding
    except Exception as error:
        record["error"] = str(error)
        raise
    finally:
        record["index_elapsed_ms"] = (time.monotonic() - index_started) * 1000
        record["total_elapsed_ms"] = (time.monotonic() - started) * 1000
        write_json(evidence_dir / "setup.json", record)


def bound_configuration(root, binary, evidence_dir):
    root, binary, evidence_dir = configuration(root, binary, evidence_dir)
    marker = absolute_path(root / ".aide" / MARKER)
    if not marker.is_file():
        raise ValueError("Fixture has not been prepared by the isolated bridge")
    binding = strict_json(marker.read_text())
    expected = {"schema_version": 1, "root": str(root), "binary": str(binary),
                "binary_sha256": sha256(binary), "evidence_dir": str(evidence_dir)}
    if not isinstance(binding, dict) or any(binding.get(k) != v for k, v in expected.items()):
        raise ValueError("Fixture binding does not match root, binary, or evidence directory")
    setup = strict_json(absolute_path(evidence_dir / "setup.json").read_text())
    if setup.get("binding") != binding or setup.get("error") is not None:
        raise ValueError("Fixture binding does not match successful controller setup")
    # aide creates this one executable symlink itself. All other runtime
    # symlinks could redirect databases, grammars, or configuration outside root.
    for directory, dirs, names in os.walk(root / ".aide", followlinks=False):
        for name in [*dirs, *names]:
            path = Path(directory) / name
            if path.is_symlink() and not (path == root / ".aide" / "bin" / "aide" and path.resolve() == binary):
                raise ValueError("Runtime symlink could escape the fixture")
    return root, binary, evidence_dir, binding


def validate_request(root, request):
    if not isinstance(request, dict) or set(request) != {"name", "arguments"}:
        raise ValueError("Tool JSON must contain exactly name and arguments")
    name, args = request["name"], request["arguments"]
    if not isinstance(name, str) or name not in TOOLS:
        raise ValueError("Only the five permitted retrieval tools are available")
    if not isinstance(args, dict) or set(args) - TOOLS[name]:
        raise ValueError("Unknown or malformed tool arguments")
    args = dict(args)
    for key in ("query", "symbol", "kind", "lang"):
        if key in args and not isinstance(args[key], str):
            raise ValueError(f"{key} must be a string")
    for key in ("limit", "start_line"):
        if key in args and (type(args[key]) is not int or args[key] > 10000):
            raise ValueError(f"{key} must be an integer no greater than 10000")
    if "keep_comments" in args and type(args["keep_comments"]) is not bool:
        raise ValueError("keep_comments must be boolean")
    if "symbols" in args and args["symbols"] is not None:
        names = args["symbols"]
        if not isinstance(names, list) or len(names) > 10 or any(not isinstance(n, str) or not n for n in names):
            raise ValueError("symbols must contain at most ten nonempty names")
    if name == "code_search" and not args.get("query"):
        raise ValueError("query is required")
    if name in {"code_references", "code_read_symbol"} and not (args.get("symbol") or args.get("symbols")):
        raise ValueError("symbol or symbols is required")
    exact = name in {"code_symbols", "code_outline", "code_read_symbol"}
    if "file" in args:
        value = args["file"]
        if not isinstance(value, str) or not value or "\x00" in value:
            raise ValueError("file must be a nonempty path or filter")
        candidate = Path(value)
        if ".." in candidate.parts or ".aide" in candidate.parts or any(p in VCS for p in candidate.parts):
            raise ValueError("Traversal and aide/VCS metadata paths are forbidden")
        if candidate.is_absolute():
            try:
                candidate = candidate.relative_to(root)
            except ValueError:
                raise ValueError("File path is outside the fixture") from None
        absolute = absolute_path(root / candidate)
        if absolute == root or root not in absolute.parents:
            raise ValueError("File path is outside the fixture")
        if exact and not absolute.is_file():
            raise ValueError("Source tools require an existing fixture file")
        args["file"] = candidate.as_posix()
    elif name in {"code_symbols", "code_outline"}:
        raise ValueError("file is required")
    return {"name": name, "arguments": args}


class RPC:
    def __init__(self, process, timeout, transcript):
        self.process, self.timeout, self.transcript = process, timeout, transcript
        self.buffer = b""
        self.selector = selectors.DefaultSelector()
        self.selector.register(process.stdout, selectors.EVENT_READ)
        self.sequence = 0

    def send(self, message):
        self.transcript.append({"direction": "request", "message": message})
        self.process.stdin.write((json.dumps(message) + "\n").encode())
        self.process.stdin.flush()

    def request(self, method, params):
        self.sequence += 1
        request_id = self.sequence
        self.send({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params})
        deadline = time.monotonic() + self.timeout
        while True:
            if b"\n" not in self.buffer:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not self.selector.select(remaining):
                    raise TimeoutError("Timed out waiting for isolated MCP")
                chunk = os.read(self.process.stdout.fileno(), 65536)
                if not chunk:
                    raise RuntimeError("Isolated MCP closed stdout before its response")
                self.buffer += chunk
                continue
            line, self.buffer = self.buffer.split(b"\n", 1)
            message = strict_json(line)
            self.transcript.append({"direction": "response", "message": message})
            if message.get("id") == request_id:
                if "error" in message:
                    raise RuntimeError(f"Isolated MCP protocol error: {message['error']}")
                if not isinstance(message.get("result"), dict):
                    raise RuntimeError("Malformed isolated MCP result")
                return message["result"]


def text_contents(result):
    content = result.get("content")
    if not isinstance(content, list):
        raise RuntimeError("MCP result has no content array")
    texts = []
    for item in content:
        if not isinstance(item, dict) or item.get("type") != "text" or not isinstance(item.get("text"), str):
            raise RuntimeError("Unexpected non-text retrieval response")
        texts.append(item["text"])
    return "".join(texts)


def call(root, binary, evidence_dir, request, timeout=30):
    started = time.monotonic()
    root, binary, evidence_dir, binding = bound_configuration(root, binary, evidence_dir)
    identifier = uuid.uuid4().hex
    record = {"operation": "call", "id": identifier, "binding_id": binding["id"],
              "request": request, "transcript": [], "result": None, "error": None,
              "timings_ms": {}, "index_policy": binding["index_policy"]}
    process, rpc = None, None
    try:
        if isinstance(request, str):
            request = strict_json(request)
        request = validate_request(root, request)
        inventory = source_inventory(root)
        record["source_changed_since_seed"] = inventory != binding["seed_files"]
        record["source_files"] = inventory
        record["forwarded_request"] = request
        record["timings_ms"]["guard"] = (time.monotonic() - started) * 1000
        with (evidence_dir / f"{identifier}.stderr").open("xb") as stderr:
            phase = time.monotonic()
            process = subprocess.Popen([str(binary), "mcp"], cwd=root, env=child_environment(root),
                                       stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=stderr)
            rpc = RPC(process, timeout, record["transcript"])
            active_phase = "startup_initialize"
            try:
                rpc.request("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                            "clientInfo": {"name": "aide-outcomes-v3-bridge", "version": "1"}})
                rpc.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
                record["timings_ms"]["startup_initialize"] = (time.monotonic() - phase) * 1000
                phase = time.monotonic()
                active_phase = "identity"
                identity_result = rpc.request("tools/call", {"name": "instance_info", "arguments": {}})
                identity = strict_json(text_contents(identity_result))
                expected_db = root / ".aide" / "memory" / "memory.db"
                expected_socket = str(root / ".aide" / "aide.sock")
                if len(expected_socket.encode()) > 100:
                    expected_socket = "/tmp/aide/" + hashlib.sha256(str(root).encode()).hexdigest()[:16] + ".sock"
                if (identity.get("project_root") != str(root) or identity.get("real_project_root") != str(root)
                        or identity.get("cwd") != str(root) or identity.get("db_path") != str(expected_db)
                        or identity.get("socket_path") != expected_socket):
                    raise RuntimeError("MCP identity is outside the prepared fixture")
                record["identity"] = identity
                record["timings_ms"]["identity"] = (time.monotonic() - phase) * 1000
                phase = time.monotonic()
                active_phase = "request"
                record["result"] = rpc.request("tools/call", request)
                record["timings_ms"]["request"] = (time.monotonic() - phase) * 1000
                text_contents(record["result"])
            finally:
                if active_phase not in record["timings_ms"]:
                    record["timings_ms"][active_phase] = (time.monotonic() - phase) * 1000
                phase = time.monotonic()
                if process.stdin:
                    try:
                        process.stdin.close()
                    except BrokenPipeError:
                        pass
                try:
                    process.wait(timeout=min(timeout, 5))
                except subprocess.TimeoutExpired:
                    record["forced_shutdown"] = True
                    process.terminate()
                    try:
                        process.wait(timeout=2)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=2)
                rpc.selector.close()
                process.stdout.close()
                record["returncode"] = process.returncode
                record["timings_ms"]["exit"] = (time.monotonic() - phase) * 1000
        if process.returncode != 0 or record.get("forced_shutdown"):
            raise RuntimeError("Isolated MCP did not exit cleanly")
        return record["result"]
    except Exception as error:
        record["error"] = str(error)
        raise
    finally:
        record["timings_ms"]["total"] = (time.monotonic() - started) * 1000
        write_json(evidence_dir / f"{identifier}.json", record)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("operation", choices=("prepare", "call"))
    parser.add_argument("--root", required=True)
    parser.add_argument("--binary", required=True)
    parser.add_argument("--evidence-dir", required=True)
    parser.add_argument("--tool-json", help='Call JSON: {"name":"code_search","arguments":{"query":"example"}}')
    parser.add_argument("--timeout", type=float, default=30)
    args = parser.parse_args()
    if not 0 < args.timeout <= 60:
        parser.error("timeout must be in (0, 60]")
    try:
        if args.operation == "prepare":
            if args.tool_json is not None:
                parser.error("prepare does not accept --tool-json")
            binding = prepare(args.root, args.binary, args.evidence_dir, args.timeout)
            print(json.dumps(binding))
        else:
            if args.tool_json is None:
                parser.error("call requires --tool-json")
            result = call(args.root, args.binary, args.evidence_dir, args.tool_json, args.timeout)
            sys.stdout.write(text_contents(result))
            return 1 if result.get("isError") else 0
    except (ValueError, RuntimeError, OSError, subprocess.SubprocessError) as error:
        print(f"isolated bridge: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

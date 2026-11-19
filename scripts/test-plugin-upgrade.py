#!/usr/bin/env python3
"""Rehearse npm/plugin upgrades on loopback with disposable Bun caches and stores.

Requires Python 3, Bun, Go, and an existing `bun install` in this checkout.
No npm publish, marketplace command, user config edit, or live store is used.
Both fixture releases use current source unless --old-binary supplies a released
binary. The marketplace download/reload itself is intentionally not simulated.
"""

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor
import copy
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import os
from pathlib import Path
import platform
import queue
import shutil
import signal
import subprocess
import tarfile
import tempfile
import threading
import time
from urllib.parse import unquote, urlsplit


ROOT = Path(__file__).resolve().parent.parent
PLUGIN = "@jmylchreest/aide-plugin"
VERSION_PACKAGE = "github.com/jmylchreest/aide/aide/internal/version"


def check(condition, message):
    if not condition:
        raise AssertionError(message)


def run(command, **kwargs):
    return subprocess.run(command, check=True, capture_output=True, text=True,
                          timeout=kwargs.pop("timeout", 120), **kwargs).stdout.strip()


def package_tarball(target, manifest, paths):
    """Build npm's package/ archive layout without invoking publish or scripts."""
    with tarfile.open(target, "w:gz", compresslevel=1, dereference=True) as archive:
        data = (json.dumps(manifest) + "\n").encode()
        header = tarfile.TarInfo("package/package.json")
        header.size, header.mode = len(data), 0o644
        archive.addfile(header, io.BytesIO(data))
        for source, destination in paths:
            def include(member):
                parts = Path(member.name).parts
                return None if "node_modules" in parts or ".git" in parts else member
            archive.add(source, arcname=f"package/{destination}", filter=include)
    return {"integrity": "sha512-" + base64.b64encode(
        hashlib.sha512(target.read_bytes()).digest()).decode()}


class Registry:
    def __init__(self, directory):
        self.directory = directory
        self.packages = {}
        self.latest = "0.2.1"
        self.unavailable = False
        self.requests = []
        registry = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                path = unquote(urlsplit(self.path).path).lstrip("/")
                if path.startswith("tarballs/"):
                    file = registry.directory / Path(path).name
                    if not file.is_file():
                        self.reply(404, b"missing tarball")
                        return
                    self.reply(200, file.read_bytes(), "application/octet-stream")
                    return
                if registry.unavailable:
                    self.reply(503, b"synthetic registry outage")
                    return
                versions = registry.packages.get(path)
                if not versions:
                    self.reply(404, b"package absent from isolated registry")
                    return
                if path.startswith("@jmylchreest/aide-"):
                    versions = {v: entry for v, entry in versions.items()
                                if v <= registry.latest}
                    latest = registry.latest
                else:
                    latest = list(versions)[-1]
                payload = {"name": path, "dist-tags": {"latest": latest},
                           "versions": versions}
                self.reply(200, json.dumps(payload).encode())

            def reply(self, status, body, mime="application/json"):
                registry.requests.append({"path": self.path, "status": status})
                self.send_response(status)
                self.send_header("Content-Type", mime)
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Cache-Control", "public, max-age=300")
                self.end_headers()
                self.wfile.write(body)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def add(self, manifest, paths):
        name, version = manifest["name"], manifest["version"]
        entries = self.packages.setdefault(name, {})
        if version in entries:
            return
        target = self.directory / f"{name.replace('/', '-').lstrip('@')}-{version}.tgz"
        metadata = copy.deepcopy(manifest)
        metadata.pop("scripts", None)
        metadata["dist"] = {**package_tarball(target, manifest, paths),
                            "tarball": f"{self.url}/tarballs/{target.name}"}
        entries[version] = metadata

    def dependency(self, name, parent):
        for ancestor in [parent, *parent.parents]:
            candidate = ancestor / "node_modules" / name / "package.json"
            if candidate.exists():
                directory = candidate.parent.resolve()
                manifest = json.loads(candidate.read_text())
                if manifest["version"] in self.packages.get(name, {}):
                    return
                paths = [(item, item.name) for item in directory.iterdir()
                         if item.name not in ("package.json", "node_modules", ".git")]
                self.add(manifest, paths)
                for dependency in manifest.get("dependencies", {}):
                    self.dependency(dependency, directory)
                return
        raise RuntimeError(f"Missing installed dependency {name}; run bun install first")

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class MCPExited(RuntimeError):
    """The launcher exited before it could answer an MCP request."""


class MCP:
    def __init__(self, command, project, env, log):
        self.log = log.open("wb")
        self.process = subprocess.Popen(command, cwd=project, env=env,
                                        stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=self.log, start_new_session=os.name != "nt")
        self.messages = queue.Queue()
        self.serial = 0
        self.closed = False

        def read():
            for line in self.process.stdout:
                try:
                    self.messages.put(json.loads(line))
                except ValueError:
                    self.messages.put(RuntimeError(f"Non-protocol stdout: {line!r}"))
            self.messages.put(MCPExited(f"MCP exited; inspect {log}"))
        self.reader = threading.Thread(target=read, daemon=True)
        self.reader.start()

    def send(self, message):
        self.process.stdin.write(json.dumps(message).encode() + b"\n")
        self.process.stdin.flush()

    def call(self, method, params):
        self.serial += 1
        self.send({"jsonrpc": "2.0", "id": self.serial, "method": method, "params": params})
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            message = self.messages.get(timeout=max(0.01, deadline-time.monotonic()))
            if isinstance(message, Exception):
                raise message
            if message.get("id") == self.serial:
                check("error" not in message, message)
                return message["result"]
        raise TimeoutError(f"MCP {method} timed out")

    def initialize(self):
        self.call("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                  "clientInfo": {"name": "aide-upgrade-rehearsal", "version": "1"}})
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return self

    def info(self):
        result = self.call("tools/call", {"name": "instance_info", "arguments": {}})
        check(not result.get("isError"), result)
        return json.loads(result["content"][0]["text"])

    def close(self):
        if self.closed:
            return
        self.closed = True
        try:
            self.process.stdin.close()
        except BrokenPipeError:
            pass
        try:
            self.process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(self.process.pid), "/T", "/F"],
                               capture_output=True, timeout=10, check=False)
            else:
                os.killpg(self.process.pid, signal.SIGKILL)
            self.process.wait(timeout=10)
        self.reader.join(timeout=2)
        self.process.stdout.close()
        self.log.close()


def build_binary(version, destination, old_binary):
    if old_binary and version == "0.2.1":
        shutil.copy2(old_binary, destination)
    else:
        flags = (f"-s -w -X {VERSION_PACKAGE}.Version={version} "
                 f"-X {VERSION_PACKAGE}.Commit=synthetic-upgrade-rehearsal "
                 f"-X {VERSION_PACKAGE}.Date=synthetic")
        run(["go", "build", "-ldflags", flags, "-o", str(destination), "./cmd/aide"],
            cwd=ROOT / "aide", timeout=600)
    check(f"version {version} " in run([str(destination), "version"]),
          f"Expected {version} binary at {destination}")


def rehearse(work, old_binary):
    project = work / "project"
    (project / ".aide").mkdir(parents=True)
    cache = work / "bun-cache"
    tarballs = work / "tarballs"
    tarballs.mkdir()
    registry = Registry(tarballs)
    clients = []
    results = []
    env = {key: value for key, value in os.environ.items()
           if key in ("PATH", "SystemRoot", "SYSTEMROOT", "WINDIR", "COMSPEC",
                      "PATHEXT", "LANG", "LC_ALL", "TEMP", "TMP")}
    npmrc = work / "empty.npmrc"
    npmrc.write_text("")
    env.update(BUN_INSTALL_CACHE_DIR=str(cache), BUN_CONFIG_REGISTRY=registry.url,
               NPM_CONFIG_USERCONFIG=str(npmrc), XDG_CONFIG_HOME=str(work / "config"),
               AIDE_PROJECT_ROOT=str(project), AIDE_CODE_WATCH="0", AIDE_DEBUG="0",
               HTTP_PROXY="http://127.0.0.1:1", HTTPS_PROXY="http://127.0.0.1:1",
               NO_PROXY="localhost,127.0.0.1")
    manifest = json.loads((ROOT / ".codex-plugin/plugin.json").read_text())
    fixture_manifest = work / "marketplace-plugin.json"

    def note(scenario, **details):
        entry = {"scenario": scenario, **details}
        results.append(entry)
        print(json.dumps(entry), flush=True)

    def launch(label, unversioned=False):
        launch_config = json.loads(fixture_manifest.read_text())["mcpServers"]["aide"]
        args = launch_config["args"]
        if unversioned:
            args = [arg.replace(PLUGIN + "@latest", PLUGIN) for arg in args]
        client = MCP([launch_config["command"], *args], project, env, work / f"{label}.log")
        clients.append(client)
        return client.initialize()

    def identity(client, version, authority=None, daemon=None):
        info = client.info()
        check(info["version"]["version"] == version, info)
        check(Path(info["project_root"]).resolve() == project.resolve(), info)
        if authority:
            check(info["authority"] == authority, info)
        if daemon:
            check(info["daemon"]["version"] == daemon, info)
            check(bool(info["daemon"].get("version_skew")) == (version != daemon), info)
        if "executable" in info:
            check(Path(info["executable"]).is_relative_to(cache), info)
        return info

    try:
        package = json.loads((ROOT / "packages/opencode-plugin/package.json").read_text())
        for dependency in package["dependencies"]:
            registry.dependency(dependency, ROOT)
        host_os = {"Linux": "linux", "Darwin": "darwin", "Windows": "windows"}[platform.system()]
        host_arch = {"x86_64": "amd64", "amd64": "amd64", "aarch64": "arm64", "arm64": "arm64"}[platform.machine().lower()]
        host_package = f"@jmylchreest/aide-binary-{host_os}-{host_arch}"
        binaries = {}
        for version in ("0.2.1", "0.2.2"):
            print(f"Preparing synthetic release {version}", flush=True)
            binary = work / (f"aide-{version}" + (".exe" if os.name == "nt" else ""))
            build_binary(version, binary, old_binary)
            binaries[version] = binary
            for path in sorted((ROOT / "packages").glob("aide-binary-*/package.json")):
                binary_package = json.loads(path.read_text())
                binary_package["version"] = version
                paths = [(binary, "bin/aide" + (".exe" if os.name == "nt" else ""))] if binary_package["name"] == host_package else []
                registry.add(binary_package, paths)
            release = copy.deepcopy(package)
            release["version"] = version
            release["optionalDependencies"] = {name: version for name in release["optionalDependencies"]}
            paths = [(ROOT / "src" / name, f"src/{name}")
                     for name in ("opencode", "core", "cli", "lib", "hooks")]
            paths += [(ROOT / "skills", "skills"), (ROOT / "bin/aide-wrapper.ts", "bin/aide-wrapper.ts")]
            registry.add(release, paths)

        # Model the installed launcher before the fix, then the new marketplace
        # manifest. No Codex command or installed marketplace is touched.
        manifest["version"] = "0.2.1"
        fixture_manifest.write_text(json.dumps(manifest))
        old_primary = launch("old-primary", unversioned=True)
        old_client = launch("old-client", unversioned=True)
        first = identity(old_primary, "0.2.1", "daemon")
        identity(old_client, "0.2.1", "client", "0.2.1")
        run([str(binaries["0.2.1"]), "state", "set", "upgrade-rehearsal", "preserved"], cwd=project, env=env)
        note("baseline", primary="0.2.1", clients=2)

        manifest["version"] = "0.2.2"
        fixture_manifest.write_text(json.dumps(manifest))
        early = launch("marketplace-before-npm")
        identity(early, "0.2.1", "client", "0.2.1")
        early.close()
        note("marketplace_before_npm", marketplace="0.2.2", loaded="0.2.1")

        registry.latest = "0.2.2"
        stale = launch("warm-unversioned", unversioned=True)
        identity(stale, "0.2.1", "client", "0.2.1")
        stale.close()
        note("warm_unversioned_cache", npm_latest="0.2.2", loaded="0.2.1")
        identity(old_primary, "0.2.1", "daemon")

        before = len(registry.requests)
        with ThreadPoolExecutor(max_workers=2) as pool:
            new_clients = list(pool.map(launch, ["new-client-a", "new-client-b"]))
        for client in new_clients:
            info = identity(client, "0.2.2", "client", "0.2.1")
            check(info["daemon"]["pid"] == first["pid"], info)
        check(any(unquote(r["path"]).lstrip("/") == PLUGIN for r in registry.requests[before:]),
              "@latest launch did not check registry metadata")
        note("latest_with_old_primary", loaded="0.2.2", primary="0.2.1", version_skew=True, clients=2)
        for client in new_clients:
            client.close()
        old_primary.close()
        deadline = time.monotonic() + 15
        while old_client.info()["authority"] != "daemon":
            check(time.monotonic() < deadline, "Old client did not take over")
            time.sleep(0.1)
        identity(old_client, "0.2.1", "daemon")
        note("restart_only_primary", surviving_primary="0.2.1")
        old_client.close()

        primary = launch("restarted-primary")
        peer = launch("restarted-peer")
        identity(primary, "0.2.2", "daemon")
        identity(peer, "0.2.2", "client", "0.2.2")
        check("preserved" in run([str(binaries["0.2.2"]), "state", "get", "upgrade-rehearsal"], cwd=project, env=env),
              "State was lost across restart")
        note("restart_all_sessions", primary="0.2.2", clients=2, version_skew=False, state_preserved=True)

        registry.unavailable = True
        try:
            offline = launch("registry-unavailable")
        except MCPExited:
            # Bun's offline policy is version-dependent. A controlled 503 may
            # fail a new launch, but must not disrupt the already-running MCPs.
            failed = clients[-1]
            check(failed.process.wait(timeout=5) != 0, "Failed launch reported success")
            check("503" in (work / "registry-unavailable.log").read_text(),
                  "Launch failed for a reason other than the simulated outage")
            note("registry_unavailable", outcome="new_launch_failed_503")
        else:
            identity(offline, "0.2.2", "client", "0.2.2")
            note("registry_unavailable", outcome="loaded_cached_0.2.2")
        identity(primary, "0.2.2", "daemon")
        identity(peer, "0.2.2", "client", "0.2.2")
        check(not any(r["status"] == 404 for r in registry.requests),
              "An undeclared package or tarball was requested")
        note("PASS", bun=run(["bun", "--version"]),
             baseline_binary="supplied release" if old_binary else "current source stamped 0.2.1",
             candidate_binary="current source stamped 0.2.2",
             plugin_sources="current checkout for both fixture versions",
             marketplace_step="fixture manifest replacement; host updater not exercised")
    finally:
        for client in reversed(clients):
            client.close()
        registry.close()
        (work / "report.json").write_text(json.dumps(results, indent=2) + "\n")
        (work / "registry-requests.json").write_text(json.dumps(registry.requests, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--old-binary", type=Path, help="Use a released 0.2.1 binary as the baseline")
    parser.add_argument("--keep", action="store_true", help="Keep fixtures, logs, and JSON report")
    args = parser.parse_args()
    work = Path(tempfile.mkdtemp(prefix="aide-upgrade-rehearsal-")).resolve()
    print(f"Isolated rehearsal: {work}", flush=True)
    succeeded = False
    try:
        rehearse(work, args.old_binary.resolve() if args.old_binary else None)
        succeeded = True
    finally:
        if succeeded and not args.keep:
            shutil.rmtree(work)
        else:
            print(f"Fixtures and logs retained: {work}", flush=True)


if __name__ == "__main__":
    main()

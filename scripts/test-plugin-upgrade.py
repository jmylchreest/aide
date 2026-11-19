#!/usr/bin/env python3
"""Rehearse npm/plugin upgrades on loopback with disposable Bun caches and stores.

Requires Python 3, Bun, Go, and an existing `bun install` in this checkout.
No publication or live plugin upgrade is performed. Native host commands use
disposable profiles and local packages; cross-host MCP sync is disabled.
Fixture binaries use current source unless --old-binary or --candidate-binary
supplies a released executable. Plugin source always comes from this checkout.
Host UI reloads are not exercised. --claude-cli exercises the installed
marketplace updater; --opencode-cli exercises the installed plugin loader.
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
import re
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
    try:
        return subprocess.run(command, check=True, capture_output=True, text=True,
                              timeout=kwargs.pop("timeout", 120), **kwargs).stdout.strip()
    except subprocess.CalledProcessError as error:
        raise RuntimeError(f"{command!r} failed ({error.returncode}):\n"
                           f"{error.stdout[-4000:]}\n{error.stderr[-4000:]}") from error


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
        self.binaries = {}
        self.github_latest = "0.2.1"
        self.github_available = {"0.2.1"}
        self.github_unavailable = False
        registry = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def do_GET(self):
                path = unquote(urlsplit(self.path).path).lstrip("/")
                if path.startswith("github/"):
                    parts = path.split("/")
                    version = registry.github_latest if "latest" in parts else parts[-2].removeprefix("v")
                    if registry.github_unavailable:
                        self.reply(503, b"synthetic GitHub outage")
                    elif version not in registry.github_available:
                        self.reply(404, b"synthetic release not published")
                    else:
                        # Give simultaneous wrappers time to contend for the lock.
                        time.sleep(0.2)
                        self.reply(200, registry.binaries[version].read_bytes(), "application/octet-stream")
                    return
                if registry.unavailable:
                    self.reply(503, b"synthetic registry outage")
                    return
                if path.startswith("tarballs/"):
                    file = registry.directory / Path(path).name
                    if not file.is_file():
                        self.reply(404, b"missing tarball")
                        return
                    self.reply(200, file.read_bytes(), "application/octet-stream")
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


def build_binary(version, destination, supplied_binary):
    if supplied_binary:
        shutil.copy2(supplied_binary, destination)
        # Downloaded release assets and CI artifacts may lose executable bits.
        destination.chmod(destination.stat().st_mode | 0o111)
    else:
        flags = (f"-s -w -X {VERSION_PACKAGE}.Version={version} "
                 f"-X {VERSION_PACKAGE}.Commit=synthetic-upgrade-rehearsal "
                 f"-X {VERSION_PACKAGE}.Date=synthetic")
        run(["go", "build", "-ldflags", flags, "-o", str(destination), "./cmd/aide"],
            cwd=ROOT / "aide", timeout=600)
    check(f"version {version} " in run([str(destination), "version"]),
          f"Expected {version} binary at {destination}")


def launch_host(command, project, env, log, clients):
    client = MCP(command, project, env, log)
    clients.append(client)
    return client.initialize()


def host_identity(client, project, version, daemon=None):
    info = client.info()
    check(info["version"]["version"] == version, info)
    check(Path(info["project_root"]).resolve() == project.resolve(), info)
    if "executable" in info:
        check(Path(info["executable"]).is_relative_to(project.parent), info)
    check(info["authority"] == ("client" if daemon else "daemon"), info)
    if daemon:
        check(info["daemon"]["version"] == daemon, info)
        check(bool(info["daemon"].get("version_skew")) == (version != daemon), info)
    return info


def rehearse_opencode(work, registry, base_env, note, clients, opencode_cli):
    project = work / "opencode-project"
    (project / ".aide").mkdir(parents=True)
    env = {**base_env, "AIDE_PROJECT_ROOT": str(project),
           "BUN_INSTALL_CACHE_DIR": str(work / "opencode-bun-cache")}
    config_file = project / "opencode.json"
    seed = {"instructions": ["KEEP.md"], "mcp": {"aide": {
        "type": "local", "enabled": True, "environment": {"KEEP": "yes"}}}}
    config_file.write_text(json.dumps(seed))

    def native_load(expected):
        # Host-specific profile variables do not redirect aide's cross-host
        # MCP sync, so disable it explicitly and guard the real config files.
        check(env.get("AIDE_MCP_SYNC") == "0", "Native probes must disable MCP sync")
        user_paths = [Path.home() / name for name in (
            ".config/opencode/opencode.json", ".claude.json", ".codex/config.toml",
            ".aide/config/mcp.json", ".aide/config/mcp-sync.journal.json")]

        def snapshot():
            return {str(path): hashlib.sha256(path.read_bytes()).hexdigest() if path.exists() else None
                    for path in user_paths}

        original = snapshot()
        host_env = {**env, **{f"XDG_{name}_HOME": str(work / "opencode-host" / name.lower())
                             for name in ("CACHE", "CONFIG", "DATA", "STATE")},
                    "npm_config_registry": registry.url, "npm_config_fetch_retries": "0",
                    "npm_config_cache": str(work / "opencode-host/npm-cache"),
                    "npm_config_fetch_timeout": "5000", "OPENCODE_DISABLE_MODELS_FETCH": "true"}
        # A host process does not inherit another host's aide package root.
        host_env.pop("AIDE_PLUGIN_ROOT", None)
        host_env.pop("CLAUDE_PLUGIN_ROOT", None)
        def probe(label):
            try:
                result = subprocess.run([opencode_cli, "--print-logs", "debug", "config"],
                                        cwd=project, env=host_env, capture_output=True, text=True, timeout=60)
            finally:
                check(snapshot() == original, "Native probe changed a real MCP configuration")
            (work / f"opencode-host-{expected}-{label}.log").write_text(result.stdout + "\n" + result.stderr)
            check(result.returncode == 0, result.stderr[-4000:])
            return result, json.loads(result.stdout)

        before = len(registry.requests)
        result, config = probe("default")
        if not config.get("skills", {}).get("paths"):
            # npm's separate HTTP metadata cache can still advertise only the
            # old release. Prove that revalidation recovers, without deleting it.
            check(not any(unquote(r["path"]).lstrip("/") == PLUGIN
                          for r in registry.requests[before:]),
                  "Plugin failed despite a fresh registry read")
            host_env["npm_config_prefer_online"] = "true"
            result, config = probe("revalidated")
            check(any(unquote(r["path"]).lstrip("/") == PLUGIN
                      for r in registry.requests[before:]), "Revalidation did not query registry")
            check(config.get("skills", {}).get("paths"), "Revalidation did not recover the plugin")
            note("opencode_metadata_cache_revalidated", plugin=expected,
                 default_load="plugin_missing", recovery="npm_config_prefer_online=true")
        check(config["mcp"]["aide"]["command"] == ["bunx", "-y", f"{PLUGIN}@{expected}", "mcp"], config)
        # debug config bootstraps the plugin and includes its registered skills.
        roots = [Path(p).parent for p in config.get("skills", {}).get("paths", [])
                 if "node_modules" in Path(p).parts and PLUGIN in p.replace("\\", "/")]
        if not roots:
            raise AssertionError(f"OpenCode did not load {PLUGIN}@{expected}; inspect "
                                 f"{work / f'opencode-host-{expected}.log'}")
        check(len(roots) == 1, roots)
        check(roots[0].is_relative_to(work / "opencode-host"), roots)
        check(json.loads((roots[0] / "package.json").read_text())["version"] == expected, roots)
        check("aide hooks init:" in result.stderr, "Host did not initialize aide")
        selected = re.search(r"aide hooks init:.*? binary=(.*?) skipInit=", result.stderr)
        check(selected and Path(selected[1]).is_relative_to(work), "Host did not select an isolated aide binary")
        check(f"version {expected} " in run([selected[1], "version"], cwd=project, env=host_env),
              f"Plugin {expected} selected a mismatched CLI: {selected[1]}")
        note("opencode_native_loader", host=run([opencode_cli, "--version"], env=host_env),
             plugin=expected, hook_binary=expected, registered_skills=True,
             real_config_unchanged=True)

    def install(version):
        output = run(["bunx", "-y", f"{PLUGIN}@{version}", "install",
                      "--platform", "opencode", "--project"], cwd=project, env=env)
        (work / f"opencode-install-{version}.log").write_text(output)
        config = json.loads(config_file.read_text())
        expected = registry.latest if version == "latest" else version
        spec = f"{PLUGIN}@{expected}"
        check(config["plugin"] == [spec], config)
        check(config["mcp"]["aide"]["command"] == ["bunx", "-y", spec, "mcp"], config)
        check(config["mcp"]["aide"]["environment"] == {"KEEP": "yes"}, config)
        check(config["instructions"] == ["KEEP.md"], config)

        # Resolve and import the configured plugin in a separate installation,
        # as a host does. This cache is independent of the bunx MCP installation.
        loader = work / "opencode-plugin-loader"
        loader.mkdir(exist_ok=True)
        (loader / "package.json").write_text(json.dumps({"dependencies": {PLUGIN: expected}}))
        run(["bun", "install", "--ignore-scripts"], cwd=loader, env=env)
        probe = loader / "probe.ts"
        probe.write_text('import plugin from "@jmylchreest/aide-plugin";\n'
                         'import {readFileSync} from "node:fs";\n'
                         'if (typeof plugin !== "function") throw new Error("plugin export missing");\n'
                         'console.log(JSON.parse(readFileSync("node_modules/@jmylchreest/aide-plugin/package.json", "utf8")).version);\n')
        check(run(["bun", str(probe)], cwd=loader, env=env) == expected,
              "Configured OpenCode plugin did not load the pinned version")
        if opencode_cli:
            native_load(expected)
        return config

    def launch(label):
        config = json.loads(config_file.read_text())
        return launch_host(config["mcp"]["aide"]["command"], project, env,
                           work / f"opencode-{label}.log", clients)

    registry.latest = "0.2.1"
    install("latest")
    old = launch("old")
    old_info = host_identity(old, project, "0.2.1")
    registry.latest = "0.2.2"
    unchanged = launch("published-only")
    host_identity(unchanged, project, "0.2.1", "0.2.1")
    unchanged.close()
    note("opencode_publish_without_install", configured="0.2.1", loaded="0.2.1")
    # Simulate invoking the installer from a still-running older host session.
    stale_root = work / "stale-opencode-plugin"
    stale_root.mkdir()
    (stale_root / "package.json").write_text(json.dumps({"version": "0.2.1"}))
    env.update(AIDE_PLUGIN_ROOT=str(stale_root), CLAUDE_PLUGIN_ROOT=str(stale_root))
    install("latest")
    new = launch("updated")
    info = host_identity(new, project, "0.2.2", "0.2.1")
    check(info["daemon"]["pid"] == old_info["pid"], info)
    note("opencode_install_latest", plugin="0.2.2", mcp="0.2.2", primary="0.2.1",
         custom_config_preserved=True, stale_root_ignored=True)
    new.close()
    old.close()
    current = launch("restarted")
    host_identity(current, project, "0.2.2")
    registry.unavailable = True
    cached = launch("pinned-offline")
    host_identity(cached, project, "0.2.2", "0.2.2")
    cached.close()
    registry.unavailable = False
    note("opencode_restart_and_cached_pin", primary="0.2.2", registry_503_launch=True)
    current.close()
    install("0.2.1")
    rollback = launch("rollback")
    host_identity(rollback, project, "0.2.1")
    rollback.close()
    note("opencode_explicit_rollback", plugin="0.2.1", mcp="0.2.1", primary="0.2.1")


def rehearse_claude(work, registry, binaries, base_env, note, clients, claude_cli):
    project = work / "claude-project"
    (project / ".aide").mkdir(parents=True)
    env = {**base_env, "AIDE_PROJECT_ROOT": str(project),
           "CLAUDE_CONFIG_DIR": str(work / "claude-config"),
           "DISABLE_AUTOUPDATER": "1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"}
    # Redirect only the transport, keeping the real wrapper, downloader, locks,
    # validation, staging and replacement. The child downloader reads bunfig too.
    preload = project / "redirect-downloads.ts"
    preload.write_text('const original = globalThis.fetch;\n'
        'globalThis.fetch = ((input, options) => {\n'
        ' const url = String(input);\n'
        ' const prefix = "https://github.com/jmylchreest/aide/";\n'
        ' if (!url.startsWith(prefix)) throw new Error(`Unexpected fetch: ${url}`);\n'
        f' return original({json.dumps(registry.url + "/github/")} + url.slice(prefix.length), options);\n'
        '}) as typeof fetch;\n')
    (project / "bunfig.toml").write_text('preload = ["./redirect-downloads.ts"]\n')
    marketplace = work / "claude-marketplace"
    source = marketplace / "plugin"
    source.mkdir(parents=True)
    for name in ("src", "skills"):
        shutil.copytree(ROOT / name, source / name)
    (source / "bin").mkdir()
    shutil.copy2(ROOT / "bin/aide-wrapper.ts", source / "bin/aide-wrapper.ts")
    manifest = json.loads((ROOT / ".claude-plugin/plugin.json").read_text())
    (source / ".claude-plugin").mkdir()
    (marketplace / ".claude-plugin").mkdir()
    package = json.loads((ROOT / "package.json").read_text())
    # Install only runtime dependencies from loopback. No npm binary packages:
    # Claude's Git distribution must exercise the GitHub download fallback.
    package = {key: package[key] for key in ("name", "version", "type", "dependencies")}

    def publish_fixture(version):
        manifest["version"] = package["version"] = version
        (source / ".claude-plugin/plugin.json").write_text(json.dumps(manifest))
        (source / "package.json").write_text(json.dumps(package))
        catalog = {"name": "aide-rehearsal", "owner": {"name": "Test"}, "plugins": [
            {"name": "aide", "version": version, "source": "./plugin"}]}
        (marketplace / ".claude-plugin/marketplace.json").write_text(json.dumps(catalog))

    def install(version, first=False):
        publish_fixture(version)
        if claude_cli:
            if first:
                run([claude_cli, "plugin", "marketplace", "add", str(marketplace)], cwd=project, env=env)
                command = [claude_cli, "plugin", "install", "aide@aide-rehearsal"]
            else:
                run([claude_cli, "plugin", "marketplace", "update", "aide-rehearsal"], cwd=project, env=env)
                command = [claude_cli, "plugin", "update", "aide@aide-rehearsal"]
            (work / f"claude-install-{version}.log").write_text(run(command, cwd=project, env=env))
            listing = json.loads(run([claude_cli, "plugin", "list", "--json"], cwd=project, env=env))
            (work / f"claude-list-{version}.json").write_text(json.dumps(listing, indent=2))
            entry = next(item for item in listing if item["id"] == "aide@aide-rehearsal")
            check(entry["version"] == version, entry)
            root = Path(entry["installPath"])
            check(root.is_relative_to(work), root)
        else:
            root = work / "claude-cache" / version
            shutil.copytree(source, root)
        run(["bun", "install", "--ignore-scripts"], cwd=root, env=env)
        return root

    def launch(root, label):
        config = json.loads((root / ".claude-plugin/plugin.json").read_text())["mcpServers"]["aide"]
        command = [config["command"], *[arg.replace("${CLAUDE_PLUGIN_ROOT}", str(root)) for arg in config["args"]]]
        return launch_host(command, project, env, work / f"claude-{label}.log", clients)

    old_root = install("0.2.1", first=True)
    old = launch(old_root, "old")
    old_info = host_identity(old, project, "0.2.1")
    new_root = install("0.2.2")
    env.update(AIDE_PLUGIN_ROOT=str(old_root), CLAUDE_PLUGIN_ROOT=str(old_root))
    binary_name = "aide.exe" if os.name == "nt" else "aide"
    destination = new_root / "bin" / binary_name
    shutil.copy2(binaries["0.2.1"], destination)
    previous = destination.read_bytes()
    try:
        launch(new_root, "release-not-ready")
        raise AssertionError("Wrapper accepted a binary older than its plugin")
    except MCPExited:
        check(destination.read_bytes() == previous, "Failed upgrade replaced the old file")
    check("older than required" in (work / "claude-release-not-ready.log").read_text(),
          "Wrapper failed for an unrelated reason")
    host_identity(old, project, "0.2.1")
    note("claude_marketplace_before_binary", new_launch="rejected_old_fallback", old_binary_preserved=True)

    registry.github_available.add("0.2.2")
    registry.github_latest = "0.2.2"
    before = len(registry.requests)
    with ThreadPoolExecutor(max_workers=2) as pool:
        new = list(pool.map(lambda label: launch(new_root, label), ["new-a", "new-b"]))
    for client in new:
        info = host_identity(client, project, "0.2.2", "0.2.1")
        check(info["daemon"]["pid"] == old_info["pid"], info)
    downloads = [r for r in registry.requests[before:] if r["path"].startswith("/github/")]
    check(len(downloads) == 1 and downloads[0]["status"] == 200, downloads)
    check(destination.read_bytes() == binaries["0.2.2"].read_bytes(), "Wrong downloaded binary")
    note("claude_concurrent_upgrade", downloads=1, loaded="0.2.2", primary="0.2.1", stale_root_ignored=True)
    for client in new:
        client.close()
    old.close()
    registry.github_unavailable = True
    before = len(registry.requests)
    current = launch(new_root, "restarted-offline")
    host_identity(current, project, "0.2.2")
    check(len(registry.requests) == before, "Warm Claude launch attempted a download")
    current.close()
    note("claude_restart_cached_binary", primary="0.2.2", github_required=False,
         marketplace="real isolated CLI" if claude_cli else "fixture cache directories")


def rehearse_codex(work, registry, binaries, env, note, clients):
    project = work / "project"
    cache = Path(env["BUN_INSTALL_CACHE_DIR"])
    manifest = json.loads((ROOT / ".codex-plugin/plugin.json").read_text())
    fixture_manifest = work / "marketplace-plugin.json"

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
    primary.close()
    peer.close()
    registry.unavailable = False


def rehearse(work, old_binary, candidate_binary, claude_cli, opencode_cli, harness):
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
               AIDE_MCP_SYNC="0",
               HTTP_PROXY="http://127.0.0.1:1", HTTPS_PROXY="http://127.0.0.1:1",
               NO_PROXY="localhost,127.0.0.1")
    def note(scenario, **details):
        entry = {"scenario": scenario, **details}
        results.append(entry)
        print(json.dumps(entry), flush=True)

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
            source = old_binary if version == "0.2.1" else candidate_binary
            build_binary(version, binary, source)
            note("binary_fixture", version=version,
                 source="supplied binary" if source else "current source",
                 identity=run([str(binary), "version"]),
                 sha256=hashlib.sha256(binary.read_bytes()).hexdigest())
            binaries[version] = binary
            registry.binaries[version] = binary
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

        if harness in ("all", "codex"):
            rehearse_codex(work, registry, binaries, env, note, clients)
        if harness in ("all", "opencode"):
            rehearse_opencode(work, registry, env, note, clients, opencode_cli)
        if harness in ("all", "claude"):
            rehearse_claude(work, registry, binaries, env, note, clients, claude_cli)
        # OpenCode bootstraps its optional authoring SDK separately. It is not
        # used by aide's runtime and is deliberately absent from this registry.
        expected_missing = {"@opencode-ai/plugin"} if opencode_cli else set()
        check(not any(r["status"] == 404 and not r["path"].startswith("/github/")
                      and unquote(r["path"]).lstrip("/") not in expected_missing
                      for r in registry.requests),
              "An undeclared package or tarball was requested")
        note("PASS", harness=harness, bun=run(["bun", "--version"]),
             baseline_binary="supplied binary" if old_binary else "current source stamped 0.2.1",
             candidate_binary="supplied binary" if candidate_binary else "current source stamped 0.2.2",
             plugin_sources="current checkout for both fixture versions",
             codex_marketplace="fixture manifest replacement" if harness in ("all", "codex") else "not selected",
             opencode_host=("native loader with authoring SDK bootstrap blocked" if opencode_cli else "separate plugin import")
             if harness in ("all", "opencode") else "not selected",
             claude_marketplace=("isolated CLI" if claude_cli else "fixture cache directories")
             if harness in ("all", "claude") else "not selected")
    except Exception as error:
        note("FAIL", harness=harness, error=str(error))
        raise
    finally:
        for client in reversed(clients):
            client.close()
        registry.close()
        (work / "report.json").write_text(json.dumps(results, indent=2) + "\n")
        (work / "registry-requests.json").write_text(json.dumps(registry.requests, indent=2) + "\n")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--harness", choices=("all", "codex", "opencode", "claude"), default="all",
                        help="Run one host independently, or all three (default)")
    parser.add_argument("--old-binary", type=Path, help="Use a released 0.2.1 binary as the baseline")
    parser.add_argument("--candidate-binary", type=Path, help="Use an existing 0.2.2 binary instead of building it")
    parser.add_argument("--report-dir", type=Path, help="Export JSON reports and logs even on failure")
    parser.add_argument("--keep", action="store_true", help="Keep fixtures, logs, and JSON report")
    parser.add_argument("--claude-cli", nargs="?", const="claude",
                        help="Also exercise the installed Claude CLI with an isolated local marketplace")
    parser.add_argument("--opencode-cli", nargs="?", const="opencode",
                        help="Also exercise the installed OpenCode loader with an isolated profile")
    args = parser.parse_args()
    if args.claude_cli and args.harness not in ("all", "claude"):
        parser.error("--claude-cli requires --harness claude or all")
    if args.opencode_cli and args.harness not in ("all", "opencode"):
        parser.error("--opencode-cli requires --harness opencode or all")
    work = Path(tempfile.mkdtemp(prefix="aide-upgrade-rehearsal-")).resolve()
    print(f"Isolated rehearsal: {work}", flush=True)
    succeeded = False
    try:
        rehearse(work, args.old_binary.resolve() if args.old_binary else None,
                 args.candidate_binary.resolve() if args.candidate_binary else None,
                 args.claude_cli, args.opencode_cli, args.harness)
        succeeded = True
    finally:
        if args.report_dir:
            args.report_dir.mkdir(parents=True, exist_ok=True)
            # Exclude caches, stores and archives: CI only needs diagnostic files.
            for path in [work / "report.json", work / "registry-requests.json", *work.glob("*.log")]:
                if path.is_file():
                    shutil.copy2(path, args.report_dir / path.name)
        if succeeded and not args.keep:
            shutil.rmtree(work)
        else:
            print(f"Fixtures and logs retained: {work}", flush=True)


if __name__ == "__main__":
    main()

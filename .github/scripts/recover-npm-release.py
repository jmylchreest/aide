#!/usr/bin/env python3
"""Prepare npm packages from checksum-verified assets of an existing release."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile


ARCHES = (
    "linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64",
    "windows-amd64", "windows-arm64",
)


def command(*args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs).strip()


def verify_digest(path, expected):
    if not re.fullmatch(r"[a-f0-9]{64}", expected):
        raise ValueError(f"Invalid checksum for {path.name}")
    with path.open("rb") as stream:
        actual = hashlib.file_digest(stream, "sha256").hexdigest()
    if actual != expected:
        raise ValueError(f"Checksum mismatch for {path.name}")


def verify_manifest(manifest, name, version, repository):
    if manifest["name"] != name or manifest["version"] != version:
        raise ValueError(f"Unexpected package identity for {name}@{version}")
    if manifest["repository"]["url"] != f"https://github.com/{repository}":
        raise ValueError(f"Unexpected repository for {name}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True)
    parser.add_argument("--repository", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", args.tag):
        parser.error("A stable release tag such as v0.2.1 is required")
    if args.output.exists():
        parser.error("The output directory must not already exist")

    version = args.tag[1:]
    commit = command("git", "rev-parse", "--verify", f"refs/tags/{args.tag}^{{commit}}")
    release = json.loads(command(
        "gh", "release", "view", args.tag, "--repo", args.repository,
        "--json", "tagName,isDraft,isPrerelease,assets",
    ))
    if release["tagName"] != args.tag or release["isDraft"] or release["isPrerelease"]:
        raise ValueError("Recovery requires an already published stable release")

    main_name = f"jmylchreest-aide-plugin-{version}.tgz"
    binaries = {arch: f"aide-{arch}" + (".exe" if arch.startswith("windows-") else "")
                for arch in ARCHES}
    required = [main_name, *binaries.values(), "checksums.txt"]
    assets = {asset["name"]: asset for asset in release["assets"]}
    for name in required:
        if name not in assets:
            raise ValueError(f"Release is missing {name}")

    with tempfile.TemporaryDirectory(prefix="aide-npm-recovery-") as temporary:
        work = Path(temporary)
        download = ["gh", "release", "download", args.tag, "--repo", args.repository,
                    "--dir", str(work)]
        for name in required:
            download.extend(["--pattern", name])
        subprocess.run(download, check=True)
        checksums = {}
        for line in (work / "checksums.txt").read_text().splitlines():
            digest, name = line.split(maxsplit=1)
            checksums[name.removeprefix("*")] = digest
        for name in required:
            # Check both the release checksum manifest and GitHub's asset digest.
            # The checksum file itself is covered by the asset digest when present.
            if name != "checksums.txt":
                verify_digest(work / name, checksums[name])
            asset_digest = assets[name].get("digest")
            if asset_digest:
                if not asset_digest.startswith("sha256:"):
                    raise ValueError(f"Unsupported asset digest for {name}")
                verify_digest(work / name, asset_digest.removeprefix("sha256:"))
            print(f"Verified {name}", flush=True)

        with tarfile.open(work / main_name) as archive:
            manifest = json.load(archive.extractfile("package/package.json"))
        verify_manifest(manifest, "@jmylchreest/aide-plugin", version, args.repository)
        expected_dependencies = {f"@jmylchreest/aide-binary-{arch}": version for arch in ARCHES}
        if manifest.get("optionalDependencies") != expected_dependencies:
            raise ValueError("Main package does not pin all six matching binary packages")

        for arch, asset in binaries.items():
            package = f"aide-binary-{arch}"
            manifest_text = command("git", "show", f"{commit}:packages/{package}/package.json")
            verify_manifest(json.loads(manifest_text), f"@jmylchreest/{package}", version, args.repository)
            staging = work / package
            (staging / "bin").mkdir(parents=True)
            (staging / "package.json").write_text(manifest_text + "\n")
            binary = staging / "bin" / ("aide.exe" if arch.startswith("windows-") else "aide")
            # Move each verified download instead of keeping duplicate large
            # binaries while packing all six platforms.
            shutil.move(work / asset, binary)
            binary.chmod(0o755)
            subprocess.run(["bun", "pm", "pack", "--ignore-scripts"], cwd=staging, check=True)
            destination = args.output / f"npm-{package}"
            destination.mkdir(parents=True)
            tarball = f"jmylchreest-{package}-{version}.tgz"
            shutil.move(staging / tarball, destination / tarball)
            shutil.rmtree(staging)

        destination = args.output / "npm-package"
        destination.mkdir(parents=True)
        shutil.copyfile(work / main_name, destination / main_name)
        print(f"Prepared seven npm packages from {args.tag} ({commit})", flush=True)


if __name__ == "__main__":
    main()

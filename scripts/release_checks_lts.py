#!/usr/bin/env python3
"""Check the Prime artifact index and signed RPM repositories without installing."""

import argparse
import gzip
import hashlib
from html.parser import HTMLParser
from pathlib import Path
import re
import subprocess
import sys
import tempfile
from urllib.parse import quote, unquote, urlsplit
import xml.etree.ElementTree as ET


NS = {"repo": "http://linux.duke.edu/metadata/repo",
      "pkg": "http://linux.duke.edu/metadata/common"}
PRIME = "https://prime.ribs.rancher.io"
VERSION = re.compile(r"v(1\.\d+)\.\d+(-rc\d+)?\+rke2r\d+")
TARGET = re.compile(r"(?:slemicro|microos|centos/(?:7|8|9|10))/(?:x86_64|aarch64)")


def run(command, timeout=180):
    result = subprocess.run(command, capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise ValueError(f"{command[0]} failed: {result.stderr.strip()}")
    return result.stdout


def fetch(url, path, debug=False):
    if debug:
        print(f"GET {url}", flush=True)
    try:
        run(["curl", "--fail", "--silent", "--show-error", "--location",
             "--proto", "=https", "--proto-redir", "=https",
             "--connect-timeout", "10", "--max-time", "120",
             "--retry", "2", "--retry-max-time", "150", "--output", str(path), url])
    except (ValueError, subprocess.TimeoutExpired) as error:
        raise ValueError(f"download {url}: {error}") from error


def child_url(base, location):
    parsed = urlsplit(location)
    decoded = unquote(parsed.path)
    if (parsed.scheme or parsed.netloc or parsed.query or parsed.fragment
            or not decoded or decoded.startswith("/") or "\\" in decoded
            or any(part in ("", ".", "..") for part in decoded.split("/"))):
        raise ValueError(f"invalid repository location: {location!r}")
    return base + location


class ArtifactIndex(HTMLParser):
    def __init__(self, version):
        super().__init__()
        self.prefix = f"{PRIME}/rke2/{version}/"
        self.files = set()

    def handle_starttag(self, tag, attrs):
        if tag != "a":
            return
        href = unquote(dict(attrs).get("href", ""))
        if href.startswith(self.prefix):
            name = href[len(self.prefix):]
            if name and "/" not in name and "?" not in name and "#" not in name:
                self.files.add(name)


def manifest_files(contents):
    files = set()
    for line in contents.splitlines():
        match = re.fullmatch(r"[0-9a-fA-F]{64} [ *]([^/\s]+)", line)
        if not match or match[1] in files:
            raise ValueError("invalid or duplicate entry in SHA256 manifest")
        files.add(match[1])
    if not files:
        raise ValueError("empty SHA256 manifest")
    return files


def verify_index(version, prerelease, directory, debug):
    index_url = PRIME + ("/index-prerelease.html" if prerelease else "/")
    index_path = directory / "index.html"
    fetch(index_url, index_path, debug)
    index = ArtifactIndex(version)
    index.feed(index_path.read_text())

    for arch in ("amd64", "arm64"):
        name = f"sha256sum-{arch}.txt"
        if name not in index.files:
            raise ValueError(f"{version}: missing {name} in Prime index")
        manifest = directory / name
        fetch(f"{PRIME}/rke2/{quote(version, safe='')}/{name}", manifest, debug)
        files = manifest_files(manifest.read_text())
        required = {f"rke2.linux-{arch}", f"rke2.linux-{arch}.tar.gz",
                    f"rke2-images.linux-{arch}.tar.zst"}
        missing = (required - files) | (files - index.files)
        if missing:
            raise ValueError(f"{version}: missing manifest/index artifacts: {sorted(missing)}")

    print(f"PASS: {version}: {len(index.files)} indexed artifacts; amd64/arm64 SHA256 "
          "manifests match the index (tarball payloads not downloaded).", flush=True)


def verify_file(path, checksum, size):
    if checksum is None or checksum.get("type") != "sha256":
        raise ValueError(f"{path.name}: missing or unsupported SHA256 checksum")
    if path.stat().st_size != int(size):
        raise ValueError(f"{path.name}: size mismatch")
    with path.open("rb") as stream:
        digest = hashlib.sha256()
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    if digest.hexdigest() != checksum.text:
        raise ValueError(f"{path.name}: SHA256 mismatch")


def repository_packages(base, directory, keyring, debug):
    directory.mkdir()
    metadata = directory / "repomd.xml"
    signature = directory / "repomd.xml.asc"
    fetch(base + "repodata/repomd.xml", metadata, debug)
    fetch(base + "repodata/repomd.xml.asc", signature, debug)
    run(["gpgv", "--homedir", str(keyring.parent), "--keyring", str(keyring),
         str(signature), str(metadata)])

    primary = ET.parse(metadata).find("repo:data[@type='primary']", NS)
    if primary is None:
        raise ValueError(f"{base}: missing primary metadata")
    location = primary.find("repo:location", NS).attrib["href"]
    path = directory / "primary.xml.gz"
    fetch(child_url(base, location), path, debug)
    verify_file(path, primary.find("repo:checksum", NS), primary.findtext("repo:size", namespaces=NS))
    with gzip.open(path, "rb") as stream:
        packages = ET.parse(stream).findall("pkg:package", NS)
    print(f"PASS: signed repository and primary metadata: {base}", flush=True)
    return packages


def release_packages(packages, version, arch):
    expected = version.removeprefix("v").replace("-", "~").replace("+", "~")
    selected = []
    for name in ("rke2-agent", "rke2-common", "rke2-server"):
        matches = [pkg for pkg in packages
                   if pkg.findtext("pkg:name", namespaces=NS) == name
                   and pkg.findtext("pkg:arch", namespaces=NS) == arch
                   and pkg.find("pkg:version", NS).get("ver") == expected]
        if len(matches) != 1:
            raise ValueError(f"expected one {name} {expected} {arch}, found {len(matches)}")
        selected.extend(matches)
    return selected


def verify_rpm(package, base, directory, debug):
    location = package.find("pkg:location", NS).attrib["href"]
    url = child_url(base, location)
    path = directory / Path(location).name
    fetch(url, path, debug)
    verify_file(path, package.find("pkg:checksum", NS), package.find("pkg:size", NS).attrib["package"])
    print(f"PASS: RPM size/SHA256: {url}", flush=True)
    path.unlink()


def verify_rpms(version, match, targets, directory, debug):
    channel = "testing" if match[2] else "latest"
    host = "https://rpm-testing.rancher.io" if match[2] else "https://rpm.rancher.io"
    key = directory / "public.key"
    keyring = directory / "keyring.gpg"
    fetch(host + "/public.key", key, debug)
    run(["gpg", "--homedir", str(directory), "--batch", "--dearmor", "--output", str(keyring), str(key)])

    checked_common = set()
    for number, target in enumerate(targets):
        distro, arch = target.rsplit("/", 1)
        base = f"{host}/rke2/{channel}/{match[1]}/{target}/prime/"
        repo_dir = directory / f"repo-{number}"
        packages = repository_packages(base, repo_dir, keyring, debug)
        for package in release_packages(packages, version, arch):
            verify_rpm(package, base, repo_dir, debug)

        if distro in checked_common:
            continue
        common = f"{host}/rke2/{channel}/common/{distro}/noarch/"
        common_dir = directory / f"common-{number}"
        policies = [pkg for pkg in repository_packages(common, common_dir, keyring, debug)
                    if pkg.findtext("pkg:name", namespaces=NS) == "rke2-selinux"
                    and pkg.findtext("pkg:arch", namespaces=NS) == "noarch"]
        if not policies:
            raise ValueError(f"{common}: missing rke2-selinux")
        # Verify the most recently built policy artifact; dependency solving needs an install test.
        policy = max(policies, key=lambda pkg: int(pkg.find("pkg:time", NS).attrib["build"]))
        verify_rpm(policy, common, common_dir, debug)
        checked_common.add(distro)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version")
    parser.add_argument("--rpm-targets", default="slemicro/x86_64")
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()
    match = VERSION.fullmatch(args.version)
    targets = list(dict.fromkeys(args.rpm_targets.split(",")))
    if not match or not targets or not all(TARGET.fullmatch(target) for target in targets):
        parser.error("provide an RKE2 release tag and comma-separated distro/architecture RPM targets")

    try:
        with tempfile.TemporaryDirectory(prefix="rke2-lts-check-") as temporary:
            directory = Path(temporary)
            verify_index(args.version, bool(match[2]), directory, args.debug)
            verify_rpms(args.version, match, targets, directory, args.debug)
    except (OSError, ValueError, KeyError, AttributeError, ET.ParseError,
            subprocess.TimeoutExpired) as error:
        print(f"FAIL: {args.version}: {error}", file=sys.stderr)
        return 1
    print("PASS: requested LTS artifact checks. Installation and tarball payload integrity were not tested.")
    return 0


if __name__ == "__main__":
    sys.exit(main())

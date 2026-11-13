import contextlib
import gzip
import hashlib
import io
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
import xml.etree.ElementTree as ET

import release_checks_lts as checks


class ReleaseChecks(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="release-check-fixture-")
        cls.home = Path(cls.temporary.name)
        cls.home.chmod(0o700)
        checks.run(["gpg", "--homedir", str(cls.home), "--batch", "--pinentry-mode", "loopback",
                    "--passphrase", "", "--quick-generate-key", "Release fixture <fixture@example.invalid>",
                    "ed25519", "sign", "0"])
        cls.public_key = subprocess.check_output(
            ["gpg", "--homedir", str(cls.home), "--armor", "--export"])

    @classmethod
    def tearDownClass(cls):
        subprocess.run(["gpgconf", "--homedir", str(cls.home), "--kill", "gpg-agent"], check=True)
        cls.temporary.cleanup()

    def fixture(self, version, target="slemicro/x86_64"):
        match = checks.VERSION.fullmatch(version)
        host = "https://rpm-testing.rancher.io" if match[2] else "https://rpm.rancher.io"
        channel = "testing" if match[2] else "latest"
        distro, arch = target.rsplit("/", 1)
        base = f"{host}/rke2/{channel}/{match[1]}/{target}/prime/"
        common = f"{host}/rke2/{channel}/common/{distro}/noarch/"
        responses = {host + "/public.key": self.public_key}
        urls = []
        for artifact_arch in ("amd64", "arm64"):
            names = [f"rke2.linux-{artifact_arch}", f"rke2.linux-{artifact_arch}.tar.gz",
                     f"rke2-images.linux-{artifact_arch}.tar.zst"]
            manifest = f"sha256sum-{artifact_arch}.txt"
            responses[f"{checks.PRIME}/rke2/{version.replace('+', '%2B')}/{manifest}"] = (
                "".join("0" * 64 + "  " + name + "\n" for name in names).encode())
            urls.extend(f'{checks.PRIME}/rke2/{version.replace("+", "%2B")}/{name}'
                        for name in names + [manifest])
        index_url = checks.PRIME + ("/index-prerelease.html" if match[2] else "/")
        responses[index_url] = "".join(f'<a href="{url}">artifact</a>' for url in urls).encode()

        rpm_version = version[1:].replace("-", "~").replace("+", "~")
        for repo, names, package_arch in [(base, ("rke2-agent", "rke2-common", "rke2-server"), arch),
                                          (common, ("rke2-selinux",), "noarch")]:
            packages = []
            for name in names:
                payload = (name + " package bytes").encode()
                responses[repo + name + ".rpm"] = payload
                digest = hashlib.sha256(payload).hexdigest()
                packages.append(f'<package><name>{name}</name><arch>{package_arch}</arch>'
                                f'<version ver="{rpm_version}" rel="0.slemicro"/>'
                                f'<checksum type="sha256">{digest}</checksum>'
                                f'<size package="{len(payload)}"/><time build="1"/>'
                                f'<location href="{name}.rpm"/></package>')
            primary = gzip.compress(('<metadata xmlns="' + checks.NS["pkg"] + '">'
                                     + "".join(packages) + '</metadata>').encode())
            metadata = (f'<repomd xmlns="{checks.NS["repo"]}"><data type="primary">'
                        '<location href="repodata/primary.xml.gz"/>'
                        f'<checksum type="sha256">{hashlib.sha256(primary).hexdigest()}</checksum>'
                        f'<size>{len(primary)}</size></data></repomd>').encode()
            responses[repo + "repodata/primary.xml.gz"] = primary
            responses[repo + "repodata/repomd.xml"] = metadata
            document = self.home / "repomd.xml"
            document.write_bytes(metadata)
            checks.run(["gpg", "--homedir", str(self.home), "--batch", "--yes", "--detach-sign",
                        "--output", str(self.home / "repomd.xml.asc"), str(document)])
            responses[repo + "repodata/repomd.xml.asc"] = (self.home / "repomd.xml.asc").read_bytes()
        return responses, base

    def invoke(self, responses, version, target="slemicro/x86_64"):
        requested = []

        def fetch(url, path, debug=False):
            requested.append(url)
            if url not in responses:
                raise ValueError(f"HTTP 404: {url}")
            path.write_bytes(responses[url])

        with patch.object(checks, "fetch", fetch), patch.object(sys, "argv", [
                "release_checks_lts.py", version, "--rpm-targets", target]), \
                contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            result = checks.main()
        return result, requested

    def test_rc_and_ga_across_rpm_targets(self):
        for version, target in [("v1.32.13-rc1+rke2r3", "slemicro/x86_64"),
                                ("v1.32.13-rc1+rke2r3", "slemicro/aarch64"),
                                ("v1.32.13+rke2r3", "centos/9/x86_64"),
                                ("v1.32.13+rke2r3", "centos/9/aarch64")]:
            with self.subTest(version=version, target=target):
                responses, base = self.fixture(version, target)
                result, requested = self.invoke(responses, version, target)
                self.assertEqual(result, 0)
                self.assertIn(base + "rke2-server.rpm", requested)
                self.assertEqual(len([url for url in requested if url.endswith(".rpm")]), 4)

    def test_bad_rpm_metadata_and_downloads_fail(self):
        version = "v1.32.13-rc1+rke2r3"
        for failure in ("404", "signature", "primary", "rpm-hash", "rpm-size"):
            with self.subTest(failure=failure):
                responses, base = self.fixture(version)
                if failure == "404":
                    del responses[base + "repodata/repomd.xml"]
                elif failure == "signature":
                    responses[base + "repodata/repomd.xml"] += b"changed"
                elif failure == "primary":
                    responses[base + "repodata/primary.xml.gz"] += b"changed"
                elif failure == "rpm-hash":
                    data = responses[base + "rke2-common.rpm"]
                    responses[base + "rke2-common.rpm"] = b"X" + data[1:]
                else:
                    responses[base + "rke2-common.rpm"] += b"changed"
                self.assertEqual(self.invoke(responses, version)[0], 1)

    def test_exact_version_arch_and_required_packages(self):
        version = "v1.32.13-rc1+rke2r3"
        responses, base = self.fixture(version)
        packages = ET.fromstring(gzip.decompress(responses[base + "repodata/primary.xml.gz"]))
        self.assertEqual(len(checks.release_packages(list(packages), version, "x86_64")), 3)
        for wrong_version, arch in [("v1.32.13-rc10+rke2r3", "x86_64"), (version, "aarch64")]:
            with self.assertRaises(ValueError):
                checks.release_packages(list(packages), wrong_version, arch)
        with self.assertRaises(ValueError):
            checks.release_packages(list(packages)[:-1], version, "x86_64")

    def test_markup_and_nearby_versions_do_not_count_as_artifacts(self):
        version = "v1.32.13-rc1+rke2r3"
        responses, _ = self.fixture(version)
        responses[checks.PRIME + "/index-prerelease.html"] = (
            responses[checks.PRIME + "/index-prerelease.html"].replace(b"rc1", b"rc10")
            + (f"<div>{version}</div>" * 100).encode())
        self.assertEqual(self.invoke(responses, version)[0], 1)

    def test_missing_artifact_or_malformed_manifest_fails(self):
        version = "v1.32.13-rc1+rke2r3"
        for content in ("", "bad checksum", "0" * 64 + "  missing.tar.gz\n"):
            with self.subTest(content=content):
                responses, _ = self.fixture(version)
                responses[f"{checks.PRIME}/rke2/{version.replace('+', '%2B')}/sha256sum-amd64.txt"] = content.encode()
                self.assertEqual(self.invoke(responses, version)[0], 1)

    def test_repository_paths_cannot_escape(self):
        for location in ("../a", "/a", "https://elsewhere/a", "//elsewhere/a", "%2e%2e/a", "a?token=x"):
            with self.subTest(location=location), self.assertRaises(ValueError):
                checks.child_url("https://rpm-testing.rancher.io/repo/", location)

    def test_download_error_and_timeout_are_not_success(self):
        for error in (ValueError("HTTP 404"), subprocess.TimeoutExpired("curl", 180)):
            with patch.object(checks, "run", side_effect=error), self.assertRaisesRegex(ValueError, "download https"):
                checks.fetch("https://rpm-testing.rancher.io/missing", self.home / "missing")


class ShellIntegration(unittest.TestCase):
    def public_check(self, version, registry_exit=0, nearby_tag=False):
        script = Path(os.environ.get("RELEASE_CHECKS_SCRIPT", Path(__file__).with_name("release_checks.sh"))).resolve()
        with tempfile.TemporaryDirectory(prefix="public-release-check-") as temporary:
            directory = Path(temporary)
            binary = directory / "bin"
            binary.mkdir()
            curl = binary / "curl"
            curl.write_text(f'''#!{sys.executable}
import json
import os
import sys
version = os.environ["FIXTURE_VERSION"]
tag = version.replace("+", "-")
url = sys.argv[-1]
if "registry.hub.docker.com" in url:
    suffixes = ["", "-linux-amd64", "-linux-arm64", "-windows-1809-amd64", "-windows-ltsc2022-amd64"]
    data = {{"results": [{{"name": tag + suffix}} for suffix in suffixes]}}
elif "rke2-packaging/releases" in url:
    data = [{{"tag_name": version + ".testing.0", "assets": list(range(60))}}]
elif "rke2-packaging/tags" in url:
    data = [{{"name": version + ".testing.0"}}]
elif "/releases/tags/" in url:
    data = {{"assets": list(range(74 if "rke2" in version else 16))}}
else:
    data = [{{"tag_name": version}}]
print(json.dumps(data))
''')
            skopeo = binary / "skopeo"
            skopeo.write_text(f'''#!{sys.executable}
import json
import os
tag = os.environ["FIXTURE_VERSION"].replace("+", "-")
if os.environ["NEARBY_TAG"] == "true":
    tag += "0"
print(json.dumps({{"Tags": [tag]}}))
raise SystemExit(int(os.environ["REGISTRY_EXIT"]))
''')
            curl.chmod(0o755)
            skopeo.chmod(0o755)
            environment = dict(os.environ, PATH=str(binary) + os.pathsep + os.environ["PATH"],
                               FIXTURE_VERSION=version, REGISTRY_EXIT=str(registry_exit),
                               NEARBY_TAG=str(nearby_tag).lower())
            return subprocess.run(["sh", str(script), "-v", version], env=environment,
                                  cwd=directory, capture_output=True, text=True, timeout=30)

    def test_public_k3s_and_rke2_rc_and_ga_still_pass(self):
        for version in ("v1.37.2-rc1+k3s1", "v1.37.2+k3s1", "v1.37.2-rc1+rke2r1", "v1.37.2+rke2r1"):
            with self.subTest(version=version):
                result = self.public_check(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_registry_partial_response_is_a_failure(self):
        result = self.public_check("v1.37.2-rc1+rke2r1", registry_exit=1)
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn("cannot list", result.stdout)

    def test_nearby_registry_version_is_not_a_match(self):
        result = self.public_check("v1.37.2-rc1+rke2r1", nearby_tag=True)
        self.assertNotEqual(result.returncode, 0, result.stdout)

    def test_lts_rejects_inflated_html_and_registry_transport_failure(self):
        script = Path(os.environ.get("RELEASE_CHECKS_SCRIPT", Path(__file__).with_name("release_checks.sh"))).resolve()
        with tempfile.TemporaryDirectory(prefix="release-check-shell-") as temporary:
            directory = Path(temporary)
            binary = directory / "bin"
            binary.mkdir()
            curl = binary / "curl"
            curl.write_text(f'''#!{sys.executable}
from pathlib import Path
import sys
content = "<div>v1.32.13-rc1+rke2r3</div>\\n" * 100
if "--output" in sys.argv:
    Path(sys.argv[sys.argv.index("--output") + 1]).write_text(content)
else:
    print(content)
''')
            skopeo = binary / "skopeo"
            skopeo.write_text(f'''#!{sys.executable}
print('{{"Tags": ["v1.32.13-rc1-rke2r3"]}}')
raise SystemExit(1)
''')
            curl.chmod(0o755)
            skopeo.chmod(0o755)
            scratch = directory / "scratch"
            scratch.mkdir()
            environment = dict(os.environ, PATH=str(binary) + os.pathsep + os.environ["PATH"], TMPDIR=str(scratch))
            result = subprocess.run(["sh", str(script), "-l", "v1.32.13-rc1+rke2r3"],
                                    env=environment, cwd=directory, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0, result.stdout)
            self.assertIn("cannot list", result.stdout)
            self.assertEqual(list(scratch.iterdir()), [])


if __name__ == "__main__":
    unittest.main()

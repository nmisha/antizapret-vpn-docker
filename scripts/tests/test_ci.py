"""Validate CI planning and local publish commands without registry writes."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = yaml.load((ROOT / ".github/workflows/build.yml").read_text(), Loader=yaml.BaseLoader)
MANIFEST = json.loads((ROOT / "versions.json").read_text())
PLAN = WORKFLOW["jobs"]["plan"]["steps"][1]["run"]
EXPRESSION = re.search(r"plan=\$\(jq[\s\S]*?--arg publish \"\$PUBLISH_REQUESTED\" '([\s\S]*?)' versions.json\)", PLAN).group(1)


class CiTests(unittest.TestCase):
    def plan(self, files=(), branch="v6-mine", publish="true", old=None):
        command = ["jq", "-c", "--arg", "hub", "nmisha", "--arg", "ghcr", "nmisha",
                   "--arg", "release", "v6-mine", "--arg", "branch", branch,
                   "--arg", "publish", publish, "--argjson", "files", json.dumps(files),
                   "--argjson", "old", json.dumps(MANIFEST if old is None else old),
                   EXPRESSION, "versions.json"]
        result = subprocess.run(command, cwd=ROOT, capture_output=True, text=True, check=True)
        return json.loads(result.stdout)

    def test_source_only_change_builds_and_publishes_fork_tags(self):
        plan = self.plan(["services/https/files/init.sh"])
        self.assertEqual(plan["services"], ["https"])
        self.assertEqual(set(plan["releases"][0]["tags"]), {
            "nmisha/antizapret-vpn-https:6.12.3", "nmisha/antizapret-vpn-https:latest",
            "ghcr.io/nmisha/antizapret-vpn-https:6.12.3", "ghcr.io/nmisha/antizapret-vpn-https:latest"})

    def test_pr_dev_and_nonrelease_dispatch_do_not_publish(self):
        for branch, publish in [("", "false"), ("dev", "true"), ("v6", "true")]:
            self.assertEqual(self.plan(["services/https/init.sh"], branch, publish)["releases"], [])

    def test_ci_or_shared_runtime_changes_rebuild_everything(self):
        for path in ["scripts/ci-bake.sh", ".github/workflows/build.yml", "services/antizapret/common/docktinel/state.sh"]:
            plan = self.plan([path])
            self.assertEqual(len(plan["services"]), len(MANIFEST["images"]))
            self.assertEqual(plan["pins"], [])

    def test_openvpn_source_rebuilds_server_and_ui(self):
        self.assertEqual(set(self.plan(["services/openvpn/files/fw-rules.sh"])["services"]), {"openvpn", "openvpn-ui"})

    def test_dispatch_builds_all_and_unchanged_push_builds_none(self):
        self.assertEqual(len(self.plan(old={"images": []})["services"]), len(MANIFEST["images"]))
        self.assertEqual(self.plan()["services"], [])

    def test_dependency_pins_use_fork(self):
        pins = self.plan(["services/https/init.sh"])["pins"]
        self.assertEqual(len(pins), 3)
        self.assertTrue(all(ref.startswith(("nmisha/", "ghcr.io/nmisha/")) for pin in pins for ref in pin["candidates"]))

    def test_bake_cache_and_local_dry_run(self):
        git_bash = Path("C:/Program Files/Git/bin/bash.exe")
        bash = str(git_bash) if git_bash.exists() else shutil.which("bash")
        env = os.environ | {"SERVICE": "https", "BUILD_ARCH": "amd64",
            "BUILD_CACHE_REPO": "ghcr.io/nmisha/antizapret-buildcache",
            "CACHE_WRITE_BRANCH": "v6-mine", "CACHE_BASE_BRANCH": "v6-mine", "OCI_DIR": "/tmp/ci-check"}
        result = subprocess.run([bash, "scripts/ci-bake.sh"], cwd=ROOT, env=env,
                                capture_output=True, text=True, check=True)
        bake = json.loads(result.stdout)
        self.assertIn("https-build-v6-mine-amd64", bake["target"]["https"]["cache-to"][0])
        self.assertEqual(bake["target"]["https"]["contexts"]["antizapret"], "target:shared-runtime")
        result = subprocess.run([bash, "scripts/docker-build.sh", "--dry-run", "-s", "https", "-v", "6.12.3"],
                                cwd=ROOT, capture_output=True, text=True, check=True)
        self.assertIn("nmisha/antizapret-vpn-https:6.12.3", result.stdout)


if __name__ == "__main__":
    unittest.main()

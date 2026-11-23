"""Check current docktinel recovery policies in disposable offline containers."""
import os
from pathlib import Path
import shlex
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]
IMAGE = os.environ.get("RECOVERY_TEST_IMAGE", "nmisha/antizapret-vpn-firewall:6.4.0-3")


class RecoveryTests(unittest.TestCase):
    def tasks(self, service):
        result = subprocess.run([
            "docker", "run", "--rm", "--network", "none", "--entrypoint", "bash",
            "--mount", f"type=bind,source={ROOT / 'services'},target=/source,readonly",
            IMAGE, "-ec", f"""
docktinel() {{ printf '%s\\n' "$*"; }}
watch_routes() {{ :; }}
wait_initialized() {{ :; }}
source /source/{service}/docktinel.sh
"""], capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        return [shlex.split(line) for line in result.stdout.splitlines()]

    def task(self, tasks, name):
        return next(task for task in tasks if task[2] == name)

    def test_coredns_discovery_failure_is_maintenance(self):
        tasks = self.tasks("coredns")
        startup = self.task(tasks, "initial-config")
        self.assertIn("restart-container", startup)
        self.assertIn("startup", startup)
        refresh = self.task(tasks, "refresh-endpoints")
        self.assertEqual(refresh[-1], "/root/config.sh")
        self.assertNotIn("restart-container", refresh)
        live = self.task(tasks, "live")
        self.assertEqual(live[1], "check")
        self.assertIn("http://127.0.0.1:8080/health", live)

    def test_adguard_api_failure_does_not_define_dns_liveness(self):
        tasks = self.tasks("adguard")
        refresh = self.task(tasks, "refresh-state")
        self.assertEqual(refresh[1], "job")
        self.assertEqual(refresh[-1], "/root/refresh-state.sh")
        self.assertNotIn("restart-container", refresh)
        live = self.task(tasks, "dns-serving")
        self.assertEqual(live[1], "check")
        self.assertEqual(live[live.index("--failures") + 1], "3")
        self.assertIn("/dev/tcp/127.0.0.1/53", " ".join(live))

    def test_firewall_startup_and_periodic_refresh_have_separate_policies(self):
        tasks = self.tasks("firewall")
        startup = self.task(tasks, "initial-rules")
        self.assertIn("restart-container", startup)
        self.assertEqual(startup[-1], "/root/initialize.sh")
        refresh = self.task(tasks, "refresh-lists")
        self.assertEqual(refresh[-1], "/root/refresh.sh")
        self.assertNotIn("restart-container", refresh)
        self.assertEqual(self.task(tasks, "live")[-1], "/root/healthcheck.sh")

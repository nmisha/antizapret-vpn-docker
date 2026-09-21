"""Run with python -m unittest discover -s services/https/tests -v.

Uses the existing HTTPS image for Caddy/OpenSSL, with current scripts mounted
read-only. Containers have no network, published ports or persistent data.
"""
import json
import os
from pathlib import Path
import subprocess
import unittest


ROOT = Path(__file__).resolve().parents[3]
IMAGE = os.environ.get("HTTPS_TEST_IMAGE", "nmisha/antizapret-vpn-https:6.7.9-3")
BASE = {
    "PROXY_DOMAIN": "web.example.org",
    "OCSERV_DOMAIN": "vpn.example.org",
    "PROXY_SERVICE_1": "Dashboard:444:dashboard:80",
    "SNI_ROUTE_1": "web.example.org:127.0.0.1:444:proxy-v2",
    "SNI_ROUTE_2": "vpn.example.org:ocserv:443:proxy-v2",
}
DOMAINS = {
    "PROXY_AUTHELIA_DOMAIN": "auth.example.org",
    "PROXY_VHOST_1": "2FAuth:twof.auth.example.org:2fauth:8000",
    "SNI_ROUTE_3": "auth.example.org:127.0.0.1:444:proxy-v2",
    "SNI_ROUTE_4": "twof.auth.example.org:127.0.0.1:444:proxy-v2",
}
SCRIPT = r"""
sh -n /source/init.sh
sh -n /source/entrypoint.sh
sh /source/init.sh >&2
sed '/^\/init.sh$/,$d' /source/entrypoint.sh > /tmp/cert-functions.sh
. /tmp/cert-functions.sh
sync_all_certificates >&2
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >&2
caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile
"""


def generate(extra, script=SCRIPT):
    env = BASE | extra
    command = ["docker", "run", "--rm", "--network", "none", "--entrypoint", "sh",
               "--add-host", "authelia:127.0.0.1",
               "--mount", f"type=bind,source={ROOT / 'services/https/files'},target=/source,readonly"]
    for key, value in env.items():
        command += ["-e", f"{key}={value}"]
    return subprocess.run(command + [IMAGE, "-ec", script], capture_output=True, text=True, encoding="utf-8", timeout=60)


def walk(value):
    if isinstance(value, dict):
        yield value
        for child in value.values():
            yield from walk(child)
    elif isinstance(value, list):
        for child in value:
            yield from walk(child)


class GeneratorTests(unittest.TestCase):
    def test_mail_vhost_uses_application_auth(self):
        config = self.config(DOMAINS | {"PROXY_VHOST_AUTH_1": "false"})
        routes = [node for node in walk(config["apps"]["http"])
                  if any(isinstance(match, dict) and match.get("host") == ["twof.auth.example.org"]
                         for match in node.get("match", [])) and "handle" in node]
        encoded = json.dumps(routes)
        self.assertIn("2fauth", encoded)
        self.assertNotIn("/api/authz/forward-auth", encoded)
        self.assertIn("Remote-*", encoded)
        # Existing port-based sites retain their authentication.
        self.assertIn("/api/authz/forward-auth", json.dumps(config))

    def test_invalid_vhost_auth_is_rejected(self):
        for extra in ({"PROXY_VHOST_AUTH_1": "flase"},
                      {"PROXY_VHOST_AUTH_1": "false",
                       "PROXY_VHOST_SHARED_GROUP_1": "shared",
                       "PROXY_VHOST_SHARED_USER_1": "mail"}):
            with self.subTest(extra=extra):
                result = generate(DOMAINS | extra)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("[ERROR]", result.stderr)

    def test_exported_mail_key_owner(self):
        script = SCRIPT + '\nstat -c "MAIL_KEY %u %a" /data/mail/certificate.key\n'
        result = generate({"SNI_CERT_1": "mail.example.org:/data/mail",
                           "SNI_CERT_UID_1": "2000"}, script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("MAIL_KEY 2000 600", result.stdout)

    def test_certificate_static_page(self):
        extra = {
            "SNI_CERT_1": "proxy.example.org:/data/telemt",
            "SNI_CERT_STATIC_1": "true",
            "SNI_ROUTE_3": "proxy.example.org:telemt:8443:proxy-v2",
            "PROXY_CERT_MODE": "selfsigned",
        }
        config = self.config(extra)
        encoded = json.dumps(config)
        self.assertIn('/srv/certificate-site', encoded)
        self.assertIn('telemt:8443', encoded)
        script = SCRIPT.replace(
            'caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile',
            '''mkdir -p /srv/certificate-site
cp /source/certificate-site/index.html /srv/certificate-site/index.html
caddy start --config /etc/caddy/Caddyfile --adapter caddyfile >&2
curl --fail --insecure --silent --show-error --noproxy '*' --max-time 5 \\
  --resolve proxy.example.org:444:127.0.0.1 https://proxy.example.org:444/
''')
        result = generate(extra, script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('<title>', result.stdout)
        self.assertIn('</html>', result.stdout)

    def test_shared_identity_requires_both_settings(self):
        for extra in [
            {"PROXY_VHOST_SHARED_GROUP_1": "shared"},
            {"PROXY_VHOST_SHARED_USER_1": "vault"},
            {"PROXY_VHOST_SHARED_GROUP_1": "shared.*", "PROXY_VHOST_SHARED_USER_1": "vault"},
            {"PROXY_VHOST_SHARED_GROUP_1": "shared", "PROXY_VHOST_SHARED_USER_1": '{http.request.header.User}'},
        ]:
            with self.subTest(extra=extra):
                result = generate(DOMAINS | extra)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("[ERROR] PROXY_VHOST_SHARED_", result.stderr)

    def test_shared_identity_after_authentication(self):
        mock = r"""
sed -i '/  servers :80 {/i\  servers :9091 {\n  }\n  servers :8000 {\n  }' /etc/caddy/Caddyfile
cat >> /etc/caddy/Caddyfile <<'EOF'
http://:9091 {
  @spoof header Remote-User attacker
  respond @spoof "Spoof reached auth" 500
  @member header X-Test-Auth member
  handle @member {
    header Remote-User alice
    header Remote-Groups "admins, twofauth_shared, all"
    header Remote-Email alice@example.org
    respond 204
  }
  @second header X-Test-Auth second
  handle @second {
    header Remote-User bob
    header Remote-Groups twofauth_shared
    header Remote-Email bob@example.org
    respond 204
  }
  @personal header X-Test-Auth personal
  handle @personal {
    header Remote-User carol
    header Remote-Groups twofauth_shared_extra
    header Remote-Email carol@example.org
    respond 204
  }
  respond "Denied" 401
}
http://:8000 {
  respond "user={http.request.header.Remote-User};email={http.request.header.Remote-Email};alias={http.request.header.Remote_User}"
}
EOF
caddy start --config /etc/caddy/Caddyfile --adapter caddyfile >&2
for mode in member second personal denied; do
  curl --insecure --silent --show-error --noproxy '*' --max-time 5 \
    --resolve 'twof.auth.example.org:443:127.0.0.1' \
    -H "X-Test-Auth: $mode" -H 'Remote-User: attacker' -H 'Remote_User: attacker' \
    -H 'Remote-Groups: twofauth_shared' -H 'Remote-Email: attacker@example.org' \
    -w '\nstatus=%{http_code}\n' https://twof.auth.example.org/
done
# A different virtual host keeps the original identity, even for group members.
curl --insecure --silent --show-error --noproxy '*' --max-time 5 \
  --resolve 'other.example.org:443:127.0.0.1' -H 'X-Test-Auth: member' \
  -w '\nstatus=%{http_code}\n' https://other.example.org/
"""
        script = SCRIPT.replace("caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile", mock)
        result = generate(DOMAINS | {
            "PROXY_CERT_MODE": "selfsigned",
            "PROXY_VHOST_1": "Shared:twof.auth.example.org:127.0.0.1:8000",
            "PROXY_VHOST_2": "Personal:other.example.org:127.0.0.1:8000",
            "SNI_ROUTE_5": "other.example.org:127.0.0.1:444:proxy-v2",
            "PROXY_VHOST_SHARED_GROUP_1": "twofauth_shared",
            "PROXY_VHOST_SHARED_USER_1": "Shared Vault",
        }, script)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout.count("user=Shared Vault;email=;alias="), 2, result.stdout + result.stderr)
        self.assertIn("user=carol;email=carol@example.org;alias=", result.stdout)
        self.assertIn("user=alice;email=alice@example.org;alias=", result.stdout)
        self.assertIn("Denied\nstatus=401", result.stdout)
        self.assertNotIn("attacker", result.stdout)

    def config(self, extra):
        result = generate(extra)
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def test_domains_and_legacy_ports(self):
        config = self.config(DOMAINS)
        servers = config["apps"]["http"]["servers"]
        https = next(server for server in servers.values() if ":444" in server["listen"])
        for domain, upstream, protected in [
            ("auth.example.org", "authelia:9091", False),
            ("twof.auth.example.org", "2fauth", True),
            ("web.example.org", "dashboard", True),
        ]:
            routes = [route for route in https["routes"]
                      if any(domain in match.get("host", []) for match in route.get("match", []))]
            self.assertEqual(len(routes), 1, domain)
            encoded = json.dumps(routes)
            self.assertIn(upstream, encoded)
            self.assertEqual("/api/authz/forward-auth" in encoded, protected)
            self.assertNotIn('"status_code": 204', encoded)
        listeners = [address for server in servers.values() for address in server["listen"]]
        self.assertNotIn(":9091", listeners)
        self.assertNotIn(":10443", listeners)
        self.assertNotIn(":443", listeners)
        encoded = json.dumps(config)
        self.assertIn('"proxy_protocol": "v2"', encoded)
        for domain in ("auth.example.org", "twof.auth.example.org"):
            self.assertIn(f"https://{domain}{{http.request.uri}}", encoded)

    def test_selfsigned(self):
        config = self.config(DOMAINS | {"PROXY_CERT_MODE": "selfsigned"})
        encoded = json.dumps(config)
        self.assertIn("/data/vhosts/auth.example.org/fallback.crt", encoded)
        self.assertIn("/data/vhosts/twof.auth.example.org/fallback.crt", encoded)
        self.assertNotIn('"module": "acme"', encoded)

    def test_exported_vhost_certificate_does_not_duplicate_site(self):
        config = self.config(DOMAINS | {"SNI_CERT_1": "twof.auth.example.org:/data/twof"})
        matches = [node for node in walk(config["apps"]["http"])
                   if node.get("host") == ["twof.auth.example.org"]]
        self.assertEqual(len(matches), 2)  # One HTTPS route and one HTTP redirect.
        self.assertIn("/data/twof/certificate.crt", json.dumps(config))

    def test_legacy_authelia(self):
        config = self.config({})
        listeners = [address for server in config["apps"]["http"]["servers"].values()
                     for address in server["listen"]]
        self.assertIn(":9091", listeners)

    def test_live_domain_redirects_and_sni(self):
        script = SCRIPT.replace("caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile", r"""
caddy start --config /etc/caddy/Caddyfile --adapter caddyfile >&2
for domain in auth.example.org twof.auth.example.org; do
    curl --fail --silent --show-error --noproxy '*' --max-time 5 \
        --resolve "$domain:80:127.0.0.1" -D - -o /dev/null "http://$domain/test?next=1"
    # No backends exist in this isolated container. A 502 proves the TLS/SNI
    # connection reached the intended HTTP reverse proxy instead of a 204 site.
    curl --insecure --silent --show-error --noproxy '*' --max-time 5 \
        --resolve "$domain:443:127.0.0.1" -o /dev/null -w '\nTLS status: %{http_code}\n' "https://$domain/"
done
""")
        result = generate(DOMAINS | {"PROXY_CERT_MODE": "selfsigned"}, script)
        self.assertEqual(result.returncode, 0, result.stderr)
        for domain in ("auth.example.org", "twof.auth.example.org"):
            self.assertIn(f"location: https://{domain}/test?next=1", result.stdout.lower())
        self.assertEqual(result.stdout.count("TLS status: 502"), 2)

    def test_dashboard_external_url(self):
        command = ["docker", "run", "--rm", "--network", "none", "--entrypoint", "sh",
                   "--mount", f"type=bind,source={ROOT / 'services/dashboard/files'},target=/source,readonly",
                   "-e", "SERVER_ROOT=/tmp", "-e", "DASHBOARD_SERVICE_1=Legacy:1443:legacy:80",
                   "-e", "DASHBOARD_SERVICE_2=2FAuth:443:2fauth:8000",
                   "-e", "DASHBOARD_SERVICE_URL_2=https://twof.auth.example.org",
                   "nmisha/antizapret-vpn-dashboard:5.0.0", "-ec",
                   "sh -n /source/init.sh; sh /source/init.sh >&2; cat /tmp/config.json"]
        result = subprocess.run(command, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        services = json.loads(result.stdout)["services"]
        self.assertEqual(services[0]["externalUrl"], "")
        self.assertEqual(services[0]["externalPort"], "1443")
        self.assertEqual(services[1]["externalUrl"], "https://twof.auth.example.org")
        self.assertEqual(services[1]["internalHostname"], "2fauth")

    def test_invalid_domain_and_routes(self):
        for extra, error in [
            ({"SNI_ROUTE_4": ""}, "Add an SNI_ROUTE_N"),
            ({"PROXY_VHOST_1": "Duplicate:auth.example.org:2fauth:8000"}, "Duplicate or reserved"),
            ({"PROXY_VHOST_1": "Invalid:bad/name.example.org:2fauth:8000"}, "Invalid virtual host domain"),
            ({"PROXY_VHOST_1": "Invalid:twof.auth.example.org:2fauth:99999"}, "invalid format"),
        ]:
            with self.subTest(extra=extra):
                result = generate(DOMAINS | extra)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(error, result.stderr)


if __name__ == "__main__":
    unittest.main()

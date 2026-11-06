"""Real AdGuard parser/DoH test on the local test Docker daemon; no published ports."""
import os
from pathlib import Path
import subprocess
import tempfile
import json

D = os.environ.get('DOCKER', '/Applications/Docker.app/Contents/Resources/bin/docker')
image = os.environ.get('NAT46_ADGUARD_IMAGE', 'antizapret-adguard-tayga:local')
names = ['n46-agh', 'n46-agh-upstream', 'n46-agh-client']
network = 'n46-agh-test'

def d(*args):
    result = subprocess.run([D, *args], text=True, capture_output=True, timeout=60)
    if result.returncode:
        raise RuntimeError(str(args) + '\n' + result.stdout + result.stderr)
    return result.stdout.strip()

if network in d('network', 'ls', '--format', '{{.Name}}').splitlines():
    raise SystemExit('Reserved AdGuard test network already exists')
try:
    d('network', 'create', network)
    for name in names[1:]:
        d('run','-d','--name',name,'--network',network,'--entrypoint','sleep','nat46-dns-tests:local','infinity')
    d('run','-d','--name',names[0],'--network',network,'--entrypoint','sleep',image,'infinity')
    address=json.loads(d('inspect', names[1]))[0]['NetworkSettings']['Networks'][network]['IPAddress']
    server='''from dnslib import RR,QTYPE,A,AAAA
from dnslib.server import DNSServer,BaseResolver
import threading
class Resolver(BaseResolver):
 def resolve(self,q,h):
  r=q.reply()
  if q.q.qtype==QTYPE.AAAA:r.add_answer(RR(q.q.qname,QTYPE.AAAA,rdata=AAAA('2001:db8::20')))
  elif q.q.qtype==QTYPE.A:r.add_answer(RR(q.q.qname,QTYPE.A,rdata=A('192.0.2.20')))
  return r
s=DNSServer(Resolver(),address='0.0.0.0',port=5300);s.start_thread();threading.Event().wait()
'''
    with tempfile.TemporaryDirectory() as directory:
        path=Path(directory)/'upstream.py';path.write_text(server)
        d('cp',str(path),names[1]+':/tmp/upstream.py')
    d('exec','-d',names[1],'python3','/tmp/upstream.py')
    source=Path(__file__).resolve().parents[2]/'services/adguard/AdGuardHome.yaml'
    d('cp',str(source),names[0]+':/tmp/test.yml')
    expression=""".users=[] | .filters=[] | .whitelist_filters=[] |
.dns.upstream_dns=[strenv(STUB_UPSTREAM)] | .dns.bootstrap_dns=[] |
.dns.cache_enabled=false | .dns.use_private_ptr_resolvers=false |
.clients.persistent[].upstreams=[strenv(STUB_UPSTREAM)] |
(.clients.persistent[] | select(.name == "coredns")).ids=["coredns"] |
.http.address="0.0.0.0:3000" | .http.doh.insecure_enabled=true |
.user_rules=["||*^$dnsrewrite=SERVFAIL,client=az-local", "||*^$dnsrewrite=SERVFAIL,client=az-world", "@@||example.com^$dnsrewrite,client=az-local|az-world", "||blocked.example^$dnsrewrite=SERVFAIL,client=az-resolver"]"""
    d('exec','-e','STUB_UPSTREAM='+address+':5300',names[0],'yq','-i',expression,'/tmp/test.yml')
    for _ in range(2):
        d('exec','-e','NAT46_ENABLED=1',names[0],'bash','/root/configure-nat46-dns.sh','/tmp/test.yml')
    count=d('exec',names[0],'yq','[.user_rules[] | select(contains("dnstype=AAAA"))] | length','/tmp/test.yml')
    assert count=='1',count
    d('exec',names[0],'mkdir','-p','/tmp/work')
    d('exec','-d',names[0],'sh','-c','exec /opt/adguardhome/AdGuardHome -c /tmp/test.yml -w /tmp/work >/tmp/agh.log 2>&1')
    test='''from dnslib import DNSRecord,RCODE
import base64,time,urllib.request
base='http://n46-agh:3000/dns-query'
def query(client,kind,name='example.com'):
 q=DNSRecord.question(name,kind);data=base64.urlsafe_b64encode(q.pack()).rstrip(b'=').decode()
 url=base+('/'+client if client else '')+'?dns='+data
 return DNSRecord.parse(urllib.request.urlopen(url,timeout=5).read())
for i in range(50):
 try:r=query('az-world','AAAA');break
 except Exception:time.sleep(.1)
else:raise RuntimeError('AdGuard did not become ready')
for client in ('az-local','az-world','az-resolver'):
 r=query(client,'AAAA');assert r.header.rcode==0 and len(r.rr)==1,(client,r)
 assert str(r.rr[0].rdata)=='2001:db8::20'
for client in ('','coredns'):
 r=query(client,'AAAA');assert r.header.rcode==0 and not r.rr,(client,r)
r=query('','A');assert str(r.rr[0].rdata)=='192.0.2.20',r
r=query('az-world','A','unlisted.example');assert r.header.rcode==RCODE.SERVFAIL,r
r=query('az-resolver','AAAA','blocked.example');assert r.header.rcode==RCODE.SERVFAIL,r
print('Real AdGuard: internal AAAA allowed, ordinary AAAA suppressed, A and exit/exclude policy preserved')
'''
    print(d('exec',names[2],'python3','-c',test))
    d('exec','-e','NAT46_ENABLED=0',names[0],'bash','/root/configure-nat46-dns.sh','/tmp/test.yml')
    assert d('exec',names[0],'yq','.dns.aaaa_disabled','/tmp/test.yml')=='true'
    assert d('exec',names[0],'yq','[.user_rules[] | select(contains("dnstype=AAAA"))] | length','/tmp/test.yml')=='0'
    print('AdGuard configuration enable/disable/idempotence OK')
finally:
    result=subprocess.run([D,'exec',names[0],'cat','/tmp/agh.log'],capture_output=True,text=True)
    if result.returncode==0:
        (Path(__file__).resolve().parents[2]/'tmp/tayga-adguard-runtime.log').write_text(result.stdout)
    for name in names:subprocess.run([D,'rm','-f',name],capture_output=True)
    subprocess.run([D,'network','rm',network],capture_output=True)

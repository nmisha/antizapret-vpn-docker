"""Exercise the repository resolver against real TAYGA/DNAT6 with a controlled DoH upstream."""
from pathlib import Path
import json
import tempfile


def check_dns(d, ex, exit_name, exit_ip):
    here = Path(__file__).resolve().parent
    for filename in ('dnsmap.py', 'nat46.py', 'nat46-control.py'):
        # The runner expects repository sources copied next to it in the VM.
        d('cp', str(here / filename), exit_name + ':/tmp/' + filename)
    ex(exit_name, 'iptables -t nat -N dnsmap; mkdir -p /root/antizapret/iptables')
    peer_ip=json.loads(d('inspect', 'n46-c2'))[0]['NetworkSettings']['Networks']['n46-lan']['IPAddress']
    upstream = '''from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import urlparse,parse_qs
import base64
from dnslib import DNSRecord,RR,A,AAAA,CNAME,QTYPE
class Handler(BaseHTTPRequestHandler):
 def do_GET(self):
  data=parse_qs(urlparse(self.path).query)['dns'][0]
  q=DNSRecord.parse(base64.urlsafe_b64decode(data+'='*(-len(data)%4))); r=q.reply()
  if q.q.qtype == QTYPE.AAAA and str(q.q.qname)=='example.com.':r.add_answer(RR(q.q.qname,QTYPE.CNAME,rdata=CNAME('cdn.example.net')))
  elif q.q.qtype == QTYPE.AAAA:r.add_answer(RR(q.q.qname,QTYPE.AAAA,rdata=AAAA('fd46:48::21' if str(q.q.qname)=='second.example.' else 'fd46:48::20')))
  else:r.add_answer(RR(q.q.qname,QTYPE.A,rdata=A('192.0.2.20')))
  data=r.pack();self.send_response(200);self.send_header('Content-Type','application/dns-message');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
HTTPServer(('0.0.0.0',3000),Handler).serve_forever()
'''
    resolver = '''import atexit,signal,threading
from pathlib import Path
from dnsmap import ProxyResolver
from nat46 import DNAT6Backend,HostPolicy
from dnslib.server import DNSServer
class Database:
 def get(self,a):return None
 def close(self):pass
Path('/tmp/asn').write_text('')
Path('/tmp/include-hosts-ipv6-custom.txt').write_text('example.com\\nsecond.example\\n')
Path('/tmp/exclude-hosts-ipv6-custom.txt').write_text('')
b=DNAT6Backend()
r=ProxyResolver('UPSTREAM',53,3000,2,'14.18.0.0/29','az-world','az-resolver','/tmp/asn','/unused',asn_database_reader=Database(),nat46_backend=b,ipv6_policy=HostPolicy('/tmp'),nat46_checkpoint=Path('/root/antizapret/iptables') / (__import__('socket').gethostname().split('.')[0] + '.nat46.rules'))
s=DNSServer(r,address='0.0.0.0',port=5353);s.start_thread()
stop=threading.Event();signal.signal(signal.SIGTERM,lambda *a:stop.set());stop.wait()
r.close();s.stop();b.save('/tmp/n46.rules','14.18.0.0/29')
'''.replace('UPSTREAM',peer_ip)
    with tempfile.TemporaryDirectory() as directory:
        for filename, data, container in [('upstream.py',upstream,'n46-c2'),('resolver.py',resolver,exit_name)]:
            path=Path(directory)/filename;path.write_text(data)
            d('cp',str(path),container+':/tmp/'+filename)
    d('exec','-d','n46-c2','sh','-c','exec python3 /tmp/upstream.py >/tmp/upstream.log 2>&1')
    ex(exit_name,'ip6tables -t nat -F nat46')
    def start():
        d('exec','-d',exit_name,'sh','-c','echo $$ >/tmp/dns.pid; exec python3 -u /tmp/resolver.py >/tmp/dns.log 2>&1')
        ex(exit_name,"for i in $(seq 1 50); do ss -lun | grep -q ':5353' && exit 0; sleep .1; done; cat /tmp/dns.log; exit 1")
    def query(name='example.com'):
        script="from dnslib import DNSRecord; r=DNSRecord.parse(DNSRecord.question('%s').send('%s',5353,timeout=5)); assert r.header.rcode==0,r; print(r.rr[0].rdata)"%(name,exit_ip)
        return d('exec','n46-c1','python3','-c',script)
    start()
    fake=query();assert fake=='14.18.0.2',fake
    ipv4_fake=query('ipv4.example');assert ipv4_fake=='14.18.0.3',ipv4_fake
    ex('n46-c1','ping -c 1 -W 2 '+fake)
    # A real IPv4 client TCP socket connects using the synthesized DNS result.
    tcp="import socket;s=socket.create_connection(('%s',443),5);s.sendall(b'dns-e2e');assert s.recv(7)==b'dns-e2e'"%fake
    d('exec','n46-c1','python3','-c',tcp)
    ex(exit_name,"test -s /root/antizapret/iptables/*.nat46.rules; kill -KILL $(cat /tmp/dns.pid); for i in $(seq 1 50); do ss -lun | grep -q ':5353' || exit 0; sleep .1; done; exit 1")
    ex(exit_name,'test ! -e /tmp/n46.rules')
    # Exercise the production startup/export controller, not a fixture-only restore.
    gateway=ex(exit_name,'ip -4 route show default').split()[2]
    settings=('NAT46_ENABLED=1 AZ_SUBNET=14.18.0.0/29 '
              'IPTABLES_SAVE_DISABLED=0 NAT46_IPV4_GATEWAY='+gateway+' ')
    ex(exit_name,'kill -TERM $(cat /tmp/tayga.pid); sleep .2; '
       'iptables -t nat -F dnsmap; ip6tables -t nat -F nat46; ip6tables -t nat -D PREROUTING -i nat46 -j nat46; '
       'ip6tables -t nat -X nat46; ip6tables -F nat46_unmapped; '
       'ip6tables -D FORWARD -i nat46 -d fd46:64::e12:0/111 -j nat46_unmapped; '
       'ip6tables -X nat46_unmapped; ip route del 14.18.0.0/15 dev nat46; ip link del nat46')
    ex(exit_name,settings+'python3 /tmp/nat46-control.py start')
    d('exec','-d',exit_name,'sh','-c','echo $$ >/tmp/tayga.pid; echo $$ >/dev/shm/.nat46.pid; exec tayga --nodetach -c /dev/shm/nat46/tayga.conf >/tmp/tayga.log 2>&1')
    start();assert query()==fake
    assert query('ipv4.example')==ipv4_fake
    assert query('second.example')=='14.18.0.4'
    assert '14.18.0.3/32' in ex(exit_name,'iptables -t nat -S dnsmap')
    d('exec','n46-c1','python3','-c',tcp)
    print('Partial CNAME -> synthetic A -> IPv6 TCP; SIGKILL/combined IPv4+IPv6 checkpoint/kernel restoration OK',flush=True)

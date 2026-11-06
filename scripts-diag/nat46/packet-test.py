import subprocess, pathlib, concurrent.futures, json, os, time
D='docker'
def d(*a):
 r=subprocess.run([D,*a],text=True,capture_output=True,timeout=90)
 if r.returncode: raise RuntimeError(f'{a}: {r.stdout} {r.stderr}')
 return r.stdout.strip()
def ex(n,cmd): return d('exec',n,'sh','-ec',cmd)
swarm=os.getenv('N46_SWARM') == '1'
nat66=os.getenv('N46_NAT66', '1') == '1'
exit_name='n46-exit'
def host(*args):
 r=subprocess.run(args,text=True,capture_output=True)
 if r.returncode: raise RuntimeError(str(args)+r.stderr)
 return r.stdout
names=['n46-exit','n46-server','n46-c1','n46-c2']; nets=['n46-lan','n46-wan']
if os.geteuid() != 0 or not pathlib.Path('/etc/antizapret-nat46-test-vm').exists():
 raise SystemExit('Run as root inside the disposable provisioned NAT46 VM only')
occupied=set(d('network','ls','--format','{{.Name}}').splitlines())
if occupied.intersection(nets+['n46-ipv6','n46-ipv6-config']):
 raise SystemExit('Test network names already exist; inspect/clean previous test first')
if d('ps','-aq','--filter','name=^n46-'):
 raise SystemExit('Test container names already exist')
try:
 if swarm:
  d('network','create','--driver','overlay','--attachable','--opt','encrypted','--subnet','172.30.46.0/24',nets[0])
  host('ip','link','add','n46-parent','type','dummy')
  host('ip','link','set','n46-parent','up')
  host('ip','link','add','n46-shim','link','n46-parent','type','macvlan','mode','bridge')
  host('ip','link','set','n46-shim','up')
  host('ip','-6','addr','add','fd46:ee::1/64','dev','n46-shim','nodad')
  d('network','create','--config-only','--ipv6','--subnet','172.31.46.0/24','--gateway','172.31.46.1','--subnet','fd46:ee::/64','--gateway','fd46:ee::1','-o','parent=n46-parent','n46-ipv6-config')
  d('network','create','-d','macvlan','--scope','swarm','--config-from','n46-ipv6-config','n46-ipv6')
 else:
  d('network','create','--subnet','172.30.46.0/24',nets[0])
 d('network','create','--ipv6','--subnet','172.30.47.0/24','--subnet','fd46:47::/64',nets[1])
 for name,addr in [('n46-c1','172.30.46.11'),('n46-c2','172.30.46.12')]:
  d('run','-d','--name',name,'--network',nets[0],'--ip',addr,'--cap-add','NET_ADMIN','nat46-jool-test:local','sleep','infinity')
 if swarm:
  d('service','create','--name','n46-exit','--no-resolve-image','--endpoint-mode','dnsrr','--cap-add','NET_ADMIN','--network',nets[0],'--network','n46-ipv6','nat46-jool-test:local','sleep','infinity')
  exit_name=d('ps','-q','--filter','name=n46-exit.')
  assert exit_name
  names[0]=exit_name
 else:
  d('run','-d','--name',exit_name,'--network',nets[0],'--ip','172.30.46.10','--cap-add','NET_ADMIN','--sysctl','net.ipv4.ip_forward=1','--sysctl','net.ipv6.conf.all.forwarding=1','nat46-jool-test:local','sleep','infinity')
  d('network','connect','--ip6','fd46:47::10',nets[1],exit_name)
 d('run','-d','--name','n46-server','--network',nets[1],'--ip','172.30.47.20','--ip6','fd46:47::20','--cap-add','NET_ADMIN','nat46-jool-test:local','sleep','infinity')
 ex('n46-server','ip -6 addr add fd46:48::20/128 dev lo')
 ex(exit_name,'sysctl -w net.ipv4.ip_forward=1 net.ipv6.conf.all.forwarding=1')
 exit_ip=json.loads(d('inspect',exit_name))[0]['NetworkSettings']['Networks'][nets[0]]['IPAddress']
 exit_v6='fd46:ee::2' if swarm else 'fd46:47::10'
 if swarm:
  actual=json.loads(d('inspect',exit_name))[0]['NetworkSettings']['Networks']['n46-ipv6']['GlobalIPv6Address']
  assert actual == exit_v6, actual
  gateway=json.loads(d('network','inspect','docker_gwbridge'))[0]['IPAM']['Config'][0]['Gateway']
  ex(exit_name,'ip route replace default via '+gateway)
 bridge='br-'+d('network','inspect','-f','{{.Id}}',nets[1])[:12]
 host('sysctl','-w','net.ipv6.conf.all.forwarding=1')
 host('ip','-6','route','add','fd46:48::/64','via','fd46:47::20','dev',bridge)
 host('ip','-6','route','add','fd46:64::/96','via',exit_v6,'dev','n46-shim' if swarm else bridge)
 if swarm:
  host('ip6tables','-I','DOCKER-USER','1','-i','n46-shim','-o',bridge,'-s','fd46:64::/96','-d','fd46:48::/64','-j','ACCEPT')
  host('ip6tables','-I','DOCKER-USER','1','-i',bridge,'-o','n46-shim','-s','fd46:48::/64','-d','fd46:64::/96','-m','conntrack','--ctstate','ESTABLISHED,RELATED','-j','ACCEPT')
 if nat66:
  host('ip6tables','-t','nat','-A','POSTROUTING','-s','fd46:64::/96','-o',bridge,'-j','MASQUERADE')
 else:
  ex('n46-server','ip -6 route add fd46:64::/96 via fd46:47::1')
 print('Jool kernel packet test',flush=True)
 pid=d('inspect','-f','{{.State.Pid}}',exit_name)
 def j(*args):
  r=subprocess.run(['nsenter','-t',pid,'-n','jool_siit',*args],text=True,capture_output=True)
  if r.returncode:raise RuntimeError(r.stdout+r.stderr)
  return r.stdout.strip()
 j('instance','add','n46','--netfilter')
 j('-i','n46','eamt','add','172.30.46.0/24','fd46:64::ac1e:2e00/120')
 from nat46 import JoolBackend
 def jool_runner(args, **kwargs):
  return subprocess.run(['nsenter','-t',pid,'-n',*args], **kwargs)
 backend=JoolBackend('n46', runner=jool_runner)
 backend.add('14.18.0.2','fd46:48::20')
 backend.add('14.18.0.2','fd46:48::20')
 backend.save('/tmp/n46-persist.rules', '14.18.0.0/15')
 assert backend.read_file('/tmp/n46-persist.rules','14.18.0.0/15') == backend.mappings()
 print('Backend confirmed add/idempotence/export validated against live Jool',flush=True)
 if not swarm: ex(exit_name,'ip -6 route add fd46:48::/64 via fd46:47::1')
 for n in names[2:]: ex(n,'ip route add 14.18.0.0/15 via '+exit_ip)
 server='''import socket,threading
s=socket.socket(socket.AF_INET6,socket.SOCK_STREAM);s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('fd46:48::20',443));s.listen()
def handle(c):
 try:
  while True:
   b=c.recv(8192)
   if not b:break
   c.sendall(b)
 finally:c.close()
def tcp():
 while True:
  c,a=s.accept();print('TCP',a,flush=True);threading.Thread(target=handle,args=(c,),daemon=True).start()
threading.Thread(target=tcp,daemon=True).start()
u=socket.socket(socket.AF_INET6,socket.SOCK_DGRAM);u.bind(('fd46:48::20',443))
while True:
 b,a=u.recvfrom(8192);print('UDP',a,flush=True);u.sendto(b,a)
'''
 p=pathlib.Path(__file__).resolve().parent/'server.py';p.write_text(server)
 d('cp',str(p),'n46-server:/tmp/server.py');d('exec','-d','n46-server','sh','-c','echo $$ >/tmp/echo.pid; exec python3 -u /tmp/server.py >/tmp/server.log 2>&1')
 client="""import socket,time
for kind in [socket.SOCK_STREAM,socket.SOCK_DGRAM]:
 s=socket.socket(socket.AF_INET,kind);s.settimeout(5);s.bind(('0.0.0.0',40000));s.connect(('14.18.0.2',443))
 for i in range(4):
  b=('test-%d'%i).encode();s.send(b);assert s.recv(100)==b;time.sleep(.3)
 s.close()
print('TCP and UDP/443 OK')
"""
 p= p.parent/'client.py';p.write_text(client)
 ex('n46-c1','ping -c 1 -W 3 14.18.0.2')
 ex('n46-server',"for i in $(seq 1 50); do ss -lnt | grep -q ':443' && exit 0; sleep .1; done; exit 1")
 for n in names[2:]:d('cp',str(p),n+':/tmp/client.py')
 with concurrent.futures.ThreadPoolExecutor() as pool:
  jobs=[pool.submit(d,'exec',n,'python3','/tmp/client.py') for n in names[2:]]
  for job in jobs:print(job.result(),flush=True)
 # Keep an established TCP connection open across map-file reload.
 hold="import socket,time;s=socket.create_connection(('14.18.0.2',443),5);s.sendall(b'before');assert s.recv(6)==b'before';open('/tmp/ready','w').close();time.sleep(3);s.sendall(b'after');assert s.recv(5)==b'after';print('existing TCP survives EAMT add')"
 with concurrent.futures.ThreadPoolExecutor() as pool:
  job=pool.submit(d,'exec','n46-c1','python3','-c',hold)
  ex('n46-c1',"for i in $(seq 1 50); do test -f /tmp/ready && exit 0; sleep .1; done; exit 1")
  j('-i','n46','eamt','add','14.18.0.3/32','fd46:48::21/128')
  print(job.result(),flush=True)
 print('ICMP:',ex('n46-c1','ping -c 2 -W 3 14.18.0.2'),flush=True)
 print('new map:',ex('n46-server','ip -6 addr add fd46:48::21/128 dev lo nodad'),flush=True)
 print('new map ping:',ex('n46-c1','ping -c 2 -W 3 14.18.0.3'),flush=True)
 host('ip','link','set',bridge,'mtu','1280')
 ex('n46-server','ip link set eth0 mtu 1280')
 print('MTU small:',ex('n46-c1','ping -c 1 -W 2 -M do -s 1200 14.18.0.2'),flush=True)
 mtu=ex('n46-c1','ping -c 1 -W 2 -M do -s 1320 14.18.0.2 || true')
 assert 'mtu = 1260' in mtu, mtu
 print('MTU DF:',mtu,flush=True)
 print('server peers:',ex('n46-server','cat /tmp/server.log'),flush=True)
 if not swarm:
  ex(exit_name,'iptables -t nat -A PREROUTING -d 14.18.0.4 -j DNAT --to-destination 172.30.47.20; iptables -t nat -A POSTROUTING -o eth1 -j MASQUERADE')
  print('IPv4 DNAT:',ex('n46-c1','ping -c 2 -W 2 14.18.0.4'),flush=True)
 if os.getenv('N46_DNS') == '1':
  from dns_integration import check_dns
  check_dns(d,ex,j,exit_name,exit_ip)
 if os.getenv('N46_QUIC') == '1':
  for n in ('n46-server','n46-c1','n46-c2'):
   d('cp',str(pathlib.Path(__file__).resolve().parent/'quic-test.py'),n+':/tmp/quic-test.py')
  ex('n46-server','kill $(cat /tmp/echo.pid); openssl req -x509 -newkey rsa:2048 -nodes -keyout /tmp/quic.key -out /tmp/quic.crt -subj /CN=nat46.test -days 1 >/dev/null 2>&1')
  d('exec','-d','n46-server','sh','-c','exec python3 /tmp/quic-test.py server >/tmp/quic.log 2>&1')
  ex('n46-server',"for i in $(seq 1 50); do ss -lun | grep -q ':443' && exit 0; sleep .1; done; cat /tmp/quic.log; exit 1")
  with concurrent.futures.ThreadPoolExecutor() as pool:
   jobs=[pool.submit(d,'exec',n,'python3','/tmp/quic-test.py','client') for n in ('n46-c1','n46-c2')]
   for job in jobs: print(job.result(),flush=True)
 print('EAMT:',j('-i','n46','eamt','display','--csv'),flush=True)
finally:
 try: print('JOOL STATS:',j('-i','n46','stats','display','--all'),flush=True)
 except Exception: pass
 for n in names[:2]:
  print(subprocess.run([D,'exec',n,'sh','-c','cat /tmp/tayga.log /tmp/server.log 2>/dev/null; ip -6 route; ip6tables -t nat -nvL; ip -s link'],capture_output=True,text=True).stdout,flush=True)
 if os.getenv('N46_KEEP'): raise SystemExit('Retained test containers for diagnostics')
 if swarm: subprocess.run([D,'service','rm','n46-exit'],capture_output=True)
 for n in names: subprocess.run([D,'rm','-f',n],capture_output=True)
 for n in reversed(nets): subprocess.run([D,'network','rm',n],capture_output=True)

 if nat66 and 'bridge' in globals():
  subprocess.run(['ip6tables','-t','nat','-D','POSTROUTING','-s','fd46:64::/96','-o',bridge,'-j','MASQUERADE'],capture_output=True)
 subprocess.run(['ip','-6','route','del','fd46:64::/96'],capture_output=True)
 subprocess.run(['ip','-6','route','del','fd46:48::/64'],capture_output=True)
 if swarm:
  for _ in range(30):
   if subprocess.run([D,'network','rm','n46-ipv6'],capture_output=True).returncode == 0:break
   time.sleep(.2)
  for _ in range(50):
   if subprocess.run([D,'network','rm','n46-ipv6-config'],capture_output=True).returncode == 0:break
   time.sleep(.2)
  subprocess.run(['ip','link','del','n46-shim'],capture_output=True)
  subprocess.run(['ip','link','del','n46-parent'],capture_output=True)

 if swarm:
  subprocess.run(['ip6tables','-D','DOCKER-USER','-i','n46-shim','-o',bridge,'-s','fd46:64::/96','-d','fd46:48::/64','-j','ACCEPT'],capture_output=True)
  subprocess.run(['ip6tables','-D','DOCKER-USER','-i',bridge,'-o','n46-shim','-s','fd46:48::/64','-d','fd46:64::/96','-m','conntrack','--ctstate','ESTABLISHED,RELATED','-j','ACCEPT'],capture_output=True)

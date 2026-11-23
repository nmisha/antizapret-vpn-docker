
#!/bin/sh

CID=$(docker ps -q --filter "label=com.docker.swarm.service.name=antizapret_wg-easy-full" | head -n1)
docker exec -it "$CID" iptables -t nat -S POSTROUTING | grep 10.8.0.0/24



docker build -t nmisha/antizapret-vpn-dashboard:5.0.0 .
docker push nmisha/antizapret-vpn-dashboard:5.0.0

docker build -t nmisha/antizapret-caddy-dav:6.0.0 .
docker push nmisha/antizapret-caddy-dav:6.0.0
docker service update --force antizapret_caddy-dav


docker build -t nmisha/antizapret-vpn-tgbot:6.0.1 .
docker push nmisha/antizapret-vpn-tgbot:6.0.1
docker service update --force antizapret_tgbot


http://az-local.antizapret/list/?file=/root/antizapret/config/custom/include-hosts-custom.txt
http://az-local.antizapret/list/?file=/root/antizapret/config/custom/include-hosts-custom.txt
http://az-local.antizapret/list/?file=/root/antizapret/config/custom/include-hosts-custom.txt



http://az-local.antizapret/list/?filter_dist=0&filter_custom=1&url=https://github.com/kablag/addPath/raw/refs/heads/main/path
http://az-local.antizapret/list/?filter_dist=0&filter_custom=1&url=https://github.com/kablag/addPath/raw/refs/heads/main/path

http://caddy-dav.antizapret/exclude-hosts-custom.txt


http://az-local.antizapret/list/?filter_dist=0&filter_custom=1&url=http://caddy-dav.antizapret/include-hosts-custom.txt
http://az-world.antizapret/list/?filter_dist=0&filter_custom=1&url=http://caddy-dav.antizapret/include-hosts-custom.txt



http://az-world.antizapret/list/?url=http://caddy-dav.antizapret/include-hosts-custom.txt


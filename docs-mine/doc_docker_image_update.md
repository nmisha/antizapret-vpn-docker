
docker image rm nmisha/antizapret-vpn-adguard:5.3.3
docker build -t nmisha/antizapret-vpn-adguard:5.3.3 .
docker push nmisha/antizapret-vpn-adguard:5.3.3

docker service update --force antizapret_wg-easy-full
docker service update --force antizapret-vpn-adguard

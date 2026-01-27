внутри контейнера антизапрот. должно отдавать ок
curl "http://az-local.antizapret/update/" -g


CRLF локальный фикс
dos2unix /root/antizapret/config/custom/exclude-hosts-custom.txt
# или
sed -i 's/\r$//' /root/antizapret/config/custom/exclude-hosts-custom.txt





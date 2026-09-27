# routes is built in services/antizapret; take it from the fork's image, not upstream.
# Build and push that image first: COPY --from pulls it from the registry.
ARG ROUTES_IMAGE=nmisha/antizapret-vpn:6.9.4-1
FROM ${ROUTES_IMAGE} AS routes

FROM adguard/adguardhome:latest

RUN apk --no-cache add iptables apache2-utils yq jq bash bind-tools curl

COPY --from=routes /usr/bin/routes /usr/bin/routes

ADD . /root/

ENTRYPOINT ["/root/entrypoint.sh"]

CMD [ \
	"--no-check-update", \
	"-c", "/opt/adguardhome/conf/AdGuardHome.yaml", \
	"-w", "/opt/adguardhome/work" \
]
FROM juvenn/caddy-dav:2.7.6

# для более удобных скриптов (можно убрать, если не нужно)
RUN apk add --no-cache bash

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
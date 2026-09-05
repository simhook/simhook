# Caddy with two extra modules:
#   caddy-dns/cloudflare      issues certificates through a DNS challenge, which
#                             works while Cloudflare proxies the site
#   caddy-cloudflare-ip       keeps Cloudflare's IP ranges as trusted proxies so
#                             the real client address reaches the API
FROM caddy:2-builder AS builder
RUN xcaddy build \
    --with github.com/caddy-dns/cloudflare \
    --with github.com/WeidiDeng/caddy-cloudflare-ip

FROM caddy:2
COPY --from=builder /usr/bin/caddy /usr/bin/caddy

ARG DISTRO=alpine:3.17.0

FROM $DISTRO as build

RUN set -euxo pipefail \
  ; apk add --no-cache go git \
  ; git clone --depth 1 https://github.com/Willow0349/alps.git \
  ; cd alps/cmd/alps \
  ; go build

FROM $DISTRO

COPY --from=build /alps/ /alps/

EXPOSE 1323/tcp
HEALTHCHECK CMD curl -skfLo /dev/null http://localhost:80/
WORKDIR /alps
USER nobody:nogroup

CMD /alps/cmd/alps/alps -theme alps -addr [::]:80 imap+insecure://mailu-front-1:10143 smtp+insecure://mailu-front-1:10025
FROM debian:latest

RUN  apt-get update \
  && apt-get install -y wget \
  && wget https://honeycomb.io/download/honeytail/v1.6.2/honeytail_1.6.2_amd64.deb \
  && echo '620e189973c8930de22d24dc7d568ac5b2a41af681f03bace69d9c6eba3c0a15  honeytail_1.6.2_amd64.deb' | sha256sum -c \
  && dpkg -i honeytail_1.6.2_amd64.deb

COPY start_honeytail.sh /start_honeytail.sh
RUN chmod 755 /start_honeytail.sh

ENTRYPOINT /start_honeytail.sh
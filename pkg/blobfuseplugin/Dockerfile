FROM ubuntu:16.04
#RUN apt-get update && apt-get install -y wget
#RUN wget -O /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/ubuntu/16.04/packages-microsoft-prod.deb
#RUN dpkg -i /tmp/packages-microsoft-prod.deb
RUN apt-get update && apt-get install -y ca-certificates pkg-config libfuse-dev cmake libcurl4-gnutls-dev libgnutls28-dev uuid-dev libgcrypt20-dev
LABEL maintainers="andyzhangx"
LABEL description="Blobfuse CSI Driver"

COPY ./_output/blobfuse /usr/bin/
COPY ./_output/blobfuseplugin /blobfuseplugin
ENTRYPOINT ["/blobfuseplugin"]

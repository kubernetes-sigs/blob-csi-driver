FROM debian:9
RUN apt-get update && apt-get install -y ca-certificates cifs-utils
LABEL maintainers="andyzhangx"
LABEL description="Blobfuse CSI Driver"

COPY ./_output/blobfuseplugin /blobfuseplugin
ENTRYPOINT ["/blobfuseplugin"]

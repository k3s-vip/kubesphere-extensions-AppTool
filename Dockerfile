FROM kubesphere/kubectl:v1.27.4
ARG TARGETARCH
COPY app-tool-${TARGETARCH} /usr/local/bin/app-tool
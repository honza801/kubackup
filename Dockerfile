# docker build . -t honza801/kubackup:v0.2
FROM alpine:3.16

COPY kubackup /usr/local/bin/
COPY k8s/config.yaml /etc/kubackup/config.yaml

ENTRYPOINT [ "/usr/local/bin/kubackup" ]

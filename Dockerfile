# docker build . -t kubackup:v0.2
FROM alpine:3.16

COPY kubackup /usr/local/bin/

ENTRYPOINT [ "/usr/local/bin/kubackup" ]

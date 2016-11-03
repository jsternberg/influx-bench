FROM golang:1.7.3-alpine

COPY . /usr/src/github.com/jsternberg/influx-bench
RUN apk add --no-cache --virtual .build-deps git && \
    go get -d -v github.com/jsternberg/influx-bench && \
    go install -v github.com/jsternberg/influx-bench && \
    apk del .build-deps
RUN cp -a /usr/src/github.com/jsternberg/influx-bench/config.toml /etc/config.toml

ENTRYPOINT ["influx-bench"]

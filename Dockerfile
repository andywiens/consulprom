FROM golang:latest as builder
WORKDIR /go/src/go.jet.network/guardians/consulprom
ADD *.go ./
RUN go get -d ./...
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-s' -installsuffix cgo -o /consulprom ./*.go

FROM prom/prometheus:latest
COPY --from=builder  /consulprom  /consulprom
WORKDIR /
COPY dockerconsulprom.yml /consulprom.yml
ENTRYPOINT ["/consulprom"]
CMD        [ "--config.file=/etc/prometheus/prometheus.yml", \
             "--storage.tsdb.path=/prometheus", \
             "--web.console.libraries=/usr/share/prometheus/console_libraries", \
             "--web.console.templates=/usr/share/prometheus/consoles" ]


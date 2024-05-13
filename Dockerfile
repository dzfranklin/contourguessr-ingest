FROM golang as build

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

RUN go build -race -o /cg-ingest .

FROM ubuntu

COPY --from=build /cg-ingest /
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["/cg-ingest"]

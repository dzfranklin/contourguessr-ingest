FROM golang

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY admin ./admin

RUN go build -o /admin ./admin

ENTRYPOINT ["/admin"]

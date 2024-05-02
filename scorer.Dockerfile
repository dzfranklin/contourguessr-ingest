FROM golang

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY scorer ./scorer

RUN go build -o /scorer ./scorer

ENTRYPOINT ["/scorer"]

FROM golang

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY flickr ./flickr
COPY flickr_indexer ./flickr_indexer

RUN go build -o /flickr_indexer ./flickr_indexer

ENTRYPOINT ["/flickr_indexer"]

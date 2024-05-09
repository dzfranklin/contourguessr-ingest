FROM golang

WORKDIR /build

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY challenge_assembler ./challenge_assembler

RUN go build -o /challenge_assembler ./challenge_assembler

ENTRYPOINT ["/challenge_assembler"]

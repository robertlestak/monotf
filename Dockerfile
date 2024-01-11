FROM golang:1.21 as builder

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /bin/monotf cmd/monotf/*.go

FROM alpine:3.6 as alpine

COPY --from=builder /bin/monotf /bin/monotf
RUN apk add -U --no-cache ca-certificates curl unzip git

ENTRYPOINT ["/bin/monotf"]
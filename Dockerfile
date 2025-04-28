FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod tidy

COPY . .

WORKDIR /app/cmd/p2p-node
RUN go build -o /main .

FROM alpine:latest

RUN apk --no-cache add ca-certificates
RUN apk --no-cache add libc6-compat

WORKDIR /root/

COPY --from=builder /main .

EXPOSE 5000

CMD ["./main"]

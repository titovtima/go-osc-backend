FROM golang:1.25.3-alpine3.22 AS builder

WORKDIR /app

COPY ./go.mod ./

COPY ./main.go ./

COPY ./channels.go ./

RUN go mod download && go mod tidy

RUN go build

FROM alpine:3.22

WORKDIR /app

COPY --from=builder /app/go-osc-backend /app/go-osc-backend

EXPOSE 8002

EXPOSE 8000

ENTRYPOINT ["/app/go-osc-backend"]

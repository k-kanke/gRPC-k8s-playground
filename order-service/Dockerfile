FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/order-service .

FROM scratch

COPY --from=builder /out/order-service /order-service

EXPOSE 50052

ENTRYPOINT ["/order-service"]

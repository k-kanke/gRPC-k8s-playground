FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/payment-service .

FROM scratch

COPY --from=builder /out/payment-service /payment-service

EXPOSE 50051

ENTRYPOINT ["/payment-service"]

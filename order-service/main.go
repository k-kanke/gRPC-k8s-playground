package main

import (
	"context"
	"log"
	"net"
	"os"

	orderpb "grpc-k8s-playground/order-service/gen"
	paymentpb "grpc-k8s-playground/order-service/paymentgen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type orderServer struct {
	orderpb.UnimplementedOrderServiceServer
	paymentClient paymentpb.PaymentServiceClient
}

func (s *orderServer) CreateOrder(ctx context.Context, req *orderpb.OrderRequest) (*orderpb.OrderResponse, error) {
	log.Printf("注文リクエスト受信: user_id=%s, item=%s, amount=%d", req.UserId, req.ItemName, req.Amount)

	// payment-serviceにgRPCで決済を依頼
	paymentRes, err := s.paymentClient.ProcessPayment(ctx, &paymentpb.PaymentRequest{
		OrderId: "order-001",
		Amount:  req.Amount,
		UserId:  req.UserId,
	})
	if err != nil {
		log.Printf("決済失敗: %v", err)
		return &orderpb.OrderResponse{
			OrderId: "order-001",
			Paid:    false,
			Message: "決済に失敗しました",
		}, nil
	}

	log.Printf("決済結果: %v", paymentRes.Message)

	return &orderpb.OrderResponse{
		OrderId: "order-001",
		Paid:    paymentRes.Success,
		Message: paymentRes.Message,
	}, nil
}

func main() {
	// payment-serviceにgRPCで接続
	paymentServiceAddr := os.Getenv("PAYMENT_SERVICE_ADDR")
	if paymentServiceAddr == "" {
		paymentServiceAddr = "localhost:50051"
	}

	conn, err := grpc.NewClient(
		paymentServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("payment-serviceに接続できません: %v", err)
	}
	defer conn.Close()

	paymentClient := paymentpb.NewPaymentServiceClient(conn)

	// order-serviceのサーバーを起動
	lis, err := net.Listen("tcp", ":50052")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(s, &orderServer{
		paymentClient: paymentClient,
	})
	reflection.Register(s)

	log.Println("order-service 起動中... port:50052")

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

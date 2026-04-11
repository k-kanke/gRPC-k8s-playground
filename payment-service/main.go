package main

import (
	"context"
	"log"
	"net"

	pb "grpc-k8s-playground/payment-service/gen"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// サーバーの構造体
type paymentServer struct {
	pb.UnimplementedPaymentServiceServer
}

// ProcessPayment の実装
func (s *paymentServer) ProcessPayment(ctx context.Context, req *pb.PaymentRequest) (*pb.PaymentResponse, error) {
	log.Printf("決済リクエスト受信: order_id=%s, amount=%d, user_id=%s", req.OrderId, req.Amount, req.UserId)

	// 今はシンプルに常に成功を返す
	return &pb.PaymentResponse{
		Success: true,
		Message: "決済が完了しました",
	}, nil
}

func main() {
	// 50051番ポートで待ち受け
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	// gRPCサーバーを作成
	s := grpc.NewServer()

	// サーバーを登録
	pb.RegisterPaymentServiceServer(s, &paymentServer{})

	reflection.Register(s)

	log.Println("payment-service 起動中... port:50051")

	// サーバー起動
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
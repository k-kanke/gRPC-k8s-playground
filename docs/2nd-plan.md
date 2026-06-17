**gRPC serviceをよりKubernetesらしく運用して挙動を理解する**

## 全体ステップ

- [ ] Step 0  現状確認
- [ ] Step 1  Service selectorとEndpointsを理解する
- [ ] Step 2  gRPC deadlineを追加する
- [ ] Step 3  payment-serviceの遅延を再現する
- [ ] Step 4  gRPC health checkを追加する
- [ ] Step 5  readinessProbeを追加する
- [ ] Step 6  livenessProbeを追加する
- [ ] Step 7  ConfigMapで接続先を管理する
- [ ] Step 8  SecretをPodに渡す
- [ ] Step 9  Rolling Updateを確認する
- [ ] Step 10 Rollbackを確認する
- [ ] Step 11 ArgoCDのselfHealとpruneを確認する

## Step 0: 現状確認

現在のアプリとArgoCDの状態を見る。

```bash
kubectl get application grpc-services -n argocd
kubectl get deploy -n grpc-app
kubectl get pods -n grpc-app -o wide
kubectl get svc -n grpc-app
```

grpcurlで今も疎通できることを確認する。

```bash
kubectl port-forward svc/order-service 50052:50052 -n grpc-app
```

別ターミナル。

```bash
grpcurl -plaintext \
  -d '{"user_id":"user-001","item_name":"book","amount":1200}' \
  localhost:50052 \
  order.OrderService/CreateOrder
```

ここで見ること。

```text
ArgoCDがSynced / Healthyか
order-service / payment-serviceのreplica数
grpcurlでorder-service -> payment-serviceまで通るか
```

## Step 1: Service selectorとEndpointsを理解する

ServiceがどのPodに通信を流しているかを見る。

```bash
kubectl get svc payment-service -n grpc-app -o yaml
kubectl get endpoints payment-service -n grpc-app
kubectl get pods -n grpc-app --show-labels
```

ここで見る関係。

```text
Service selector
  ↓
Pod labels
  ↓
Endpoints
```

実験として、manifest上でServiceのselectorを一時的に壊す。

```yaml
selector:
  app: wrong-payment-service
```

GitにpushしてArgoCDで同期されたあとに確認する。

```bash
kubectl get endpoints payment-service -n grpc-app
grpcurl -plaintext \
  -d '{"user_id":"user-001","item_name":"book","amount":1200}' \
  localhost:50052 \
  order.OrderService/CreateOrder
```

確認したらselectorを元に戻す。

```yaml
selector:
  app: payment-service
```

ここで理解したいこと。

```text
ServiceはPod名ではなくlabelでPodを見つける
selectorが合わないとServiceは存在しても転送先がなくなる
```

## Step 2: gRPC deadlineを追加する

order-serviceからpayment-serviceを呼ぶときにtimeoutを設定する。

イメージ。

```go
ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
defer cancel()
```

ここで理解したいこと。

```text
gRPC呼び出しは無制限に待たせない
呼び出し元がdeadlineを決める
deadlineを超えるとDeadlineExceededになる
```

確認。

```bash
go test ./...
docker build -t order-service:local order-service
kind load docker-image order-service:local --name grpc-lab
```

manifestに変更がある場合はGitにpushしてArgoCDで反映する。

## Step 3: payment-serviceの遅延を再現する

payment-serviceに意図的なsleepを入れて、deadlineが効くことを見る。

イメージ。

```go
time.Sleep(3 * time.Second)
```

order-serviceのdeadlineが2秒なら、payment-serviceの処理完了前に失敗する。

確認。

```bash
grpcurl -plaintext \
  -d '{"user_id":"user-001","item_name":"book","amount":1200}' \
  localhost:50052 \
  order.OrderService/CreateOrder
```

ここで理解したいこと。

```text
依存先が遅いと呼び出し元も遅くなる
deadlineで待ち時間を制限できる
```

確認後、sleepは削除する。

## Step 4: gRPC health checkを追加する

payment-serviceとorder-serviceにgRPC health checking serviceを追加する。

使うpackage。

```go
google.golang.org/grpc/health
google.golang.org/grpc/health/grpc_health_v1
```

確認。

```bash
grpcurl -plaintext localhost:50051 grpc.health.v1.Health/Check
grpcurl -plaintext localhost:50052 grpc.health.v1.Health/Check
```

ここで理解したいこと。

```text
アプリが自分の健康状態をgRPC APIとして返す
Kubernetesのprobeからも利用できる
```

## Step 5: readinessProbeを追加する

DeploymentにreadinessProbeを追加する。

イメージ。

```yaml
readinessProbe:
  grpc:
    port: 50051
  initialDelaySeconds: 3
  periodSeconds: 5
```

確認。

```bash
kubectl describe pod <pod-name> -n grpc-app
kubectl get endpoints payment-service -n grpc-app
```

ここで理解したいこと。

```text
readinessProbeが失敗しているPodにはServiceが通信を流さない
PodがRunningでもReadyでなければServiceの転送先にならない
```

## Step 6: livenessProbeを追加する

DeploymentにlivenessProbeを追加する。

イメージ。

```yaml
livenessProbe:
  grpc:
    port: 50051
  initialDelaySeconds: 10
  periodSeconds: 10
```

確認。

```bash
kubectl describe pod <pod-name> -n grpc-app
kubectl get pods -n grpc-app -w
```

ここで理解したいこと。

```text
livenessProbeが失敗するとkubeletがコンテナを再起動する
readinessProbeは通信対象から外す
livenessProbeは再起動判断に使う
```

## Step 7: ConfigMapで接続先を管理する

`PAYMENT_SERVICE_ADDR` をDeployment直書きからConfigMapに移す。

`k8s/order-configmap.yaml`

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: order-service-config
  namespace: grpc-app
data:
  PAYMENT_SERVICE_ADDR: payment-service:50051
```

Deployment側。

```yaml
envFrom:
  - configMapRef:
      name: order-service-config
```

ここで理解したいこと。

```text
環境ごとに変わる設定をmanifestから分離する
ConfigMapは機密ではない設定に使う
```

## Step 8: SecretをPodに渡す

仮のAPI keyをSecretとして作り、Podに環境変数として渡す。

```bash
kubectl create secret generic payment-api-secret \
  --from-literal=PAYMENT_API_KEY=dummy-key \
  -n grpc-app \
  --dry-run=client -o yaml
```

manifest化してGit管理する場合は、実際の秘密値をそのまま置かないこと。

ここで理解したいこと。

```text
Secretは機密値をPodに渡す仕組み
平文SecretをGitに置くのは本番では避ける
```

## Step 9: Rolling Updateを確認する

レスポンスメッセージを変更して新しいimage tagを作る。

```bash
docker build -t payment-service:v2 payment-service
kind load docker-image payment-service:v2 --name grpc-lab
```

manifestを変更する。

```yaml
image: payment-service:v2
imagePullPolicy: Never
```

GitにpushしてArgoCDで同期し、rolloutを見る。

```bash
kubectl rollout status deployment/payment-service -n grpc-app
kubectl get rs -n grpc-app
kubectl get pods -n grpc-app -w
```

ここで理解したいこと。

```text
Deploymentは新しいReplicaSetを作る
古いPodから新しいPodへ段階的に入れ替える
```

## Step 10: Rollbackを確認する

Deploymentのrollout履歴を見る。

```bash
kubectl rollout history deployment/payment-service -n grpc-app
```

GitOpsでは、基本的にはGitを戻してrollbackする。

```bash
git revert <commit>
git push origin main
```

ArgoCDが同期したあと確認する。

```bash
kubectl rollout status deployment/payment-service -n grpc-app
grpcurl -plaintext \
  -d '{"user_id":"user-001","item_name":"book","amount":1200}' \
  localhost:50052 \
  order.OrderService/CreateOrder
```

ここで理解したいこと。

```text
GitOpsではGitの履歴がrollback手段になる
クラスタを直接戻すよりGitを戻す方が状態を保ちやすい
```

## Step 11: ArgoCDのselfHealとpruneを確認する

selfHealを確認する。

```bash
kubectl scale deployment order-service --replicas=1 -n grpc-app
kubectl get deploy order-service -n grpc-app -w
```

Git上のreplicasが2なら、ArgoCDが2に戻す。

pruneを確認する。

一時的なmanifestをGitに追加してpushする。

```text
k8s/tmp-configmap.yaml
```

ArgoCDで作成されたことを確認したあと、そのmanifestをGitから削除してpushする。

```bash
kubectl get configmap -n grpc-app
```

ここで理解したいこと。

```text
selfHealはクラスタ側の手動変更をGitの状態へ戻す
pruneはGitから消えたリソースをクラスタからも削除する
```

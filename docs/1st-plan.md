> **kind上にArgoCDを入れて、GitHub管理のmanifestからgrpc-serverをデプロイする**

## 全体ステップ

- [x] Step 0  現状確認
- [x] Step 1  namespace整理
- [x] Step 2  手動でnginx Deploymentを作る
- [x] Step 3  Serviceを作って通信確認
- [x] Step 4  grpc-serverをGoで作る
- [x] Step 5  Docker imageを作る
- [ ] Step 6  kindにimageを読み込ませる
- [ ] Step 7  Kubernetes manifestを書く
- [ ] Step 8  kubectl applyで手動デプロイ
- [ ] Step 9  ArgoCDをインストール
- [ ] Step 10 ArgoCD Applicationを作る
- [ ] Step 11 GitOpsでgrpc-serverを管理する

## Step 0: 現状確認

```bash
kubectl get nodes
kubectl get pods -A -o wide
kubectl get deploy -A
kubectl get svc -A
```

ここで見ること

```text
NodeがReadyか
kube-systemのPodがRunningか
defaultにnginx-testが残っているか
```

## Step 1: namespaceを作る

```bash
kubectl create namespace grpc-app
kubectl create namespace argocd
```

確認。

```bash
kubectl get ns
```

構成イメージ。

```text
Cluster
├── kube-system
├── argocd
└── grpc-app
```

## Step 2: Deploymentを理解する

まず練習でnginxをDeploymentとして作る。

```bash
kubectl create deployment nginx-deploy \
  --image=nginx \
  --replicas=3 \
  -n grpc-app
```

確認。

```bash
kubectl get deploy -n grpc-app
kubectl get rs -n grpc-app
kubectl get pods -n grpc-app -o wide
```

ここで見る関係。

```text
Deployment
  ↓
ReplicaSet
  ↓
Pod x 3
```

## Step 3: Podを消して自動復旧を見る

```bash
kubectl delete pod <nginxのPod名> -n grpc-app
```

確認。

```bash
kubectl get pods -n grpc-app
```

消しても新しいPodが作られればOK

ここで、

```text
Podは使い捨て
Deploymentが本体
```

を理解します。

## Step 4: Serviceを作る

```bash
kubectl expose deployment nginx-deploy \
  --port=123 \
  --target-port=80 \
  -n grpc-app
```

確認。

```bash
kubectl get svc -n grpc-app
```

port-forward。

```bash
kubectl port-forward svc/nginx-deploy 8080:80 -n grpc-app
```

別ターミナルで確認。

```bash
curl localhost:8080
```

ここで、

```text
Service
  ↓
複数Podへの固定入口
```

を理解します。

## Step 5: nginx練習を削除

```bash
kubectl delete deployment nginx-deploy -n grpc-app
kubectl delete service nginx-deploy -n grpc-app
```

## Step 6: Go gRPC serverを作る

ディレクトリ例。

```text
grpc-k8s-playground
├── server
│   ├── main.go
│   ├── go.mod
│   └── proto
├── k8s
│   ├── deployment.yaml
│   └── service.yaml
└── argocd
    └── application.yaml
```

最初は超単純なgRPCでOKです。

```text
SayHello(name) -> Hello, name
```

## Step 7: Docker imageを作る

```bash
docker build -t grpc-server:local ./server
```

kindに読み込ませる。

```bash
kind load docker-image grpc-server:local --name grpc-lab
```

ここが重要です。

kindのNodeはDockerコンテナなので、ローカルでbuildしたimageをkind側に渡す必要があります。

## Step 8: manifestを書く

`k8s/deployment.yaml`

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grpc-server
  namespace: grpc-app
spec:
  replicas: 3
  selector:
    matchLabels:
      app: grpc-server
  template:
    metadata:
      labels:
        app: grpc-server
    spec:
      containers:
        - name: grpc-server
          image: grpc-server:local
          imagePullPolicy: Never
          ports:
            - containerPort: 50051
```

`k8s/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: grpc-server
  namespace: grpc-app
spec:
  type: ClusterIP
  selector:
    app: grpc-server
  ports:
    - name: grpc
      port: 50051
      targetPort: 50051
```

適用。

```bash
kubectl apply -f k8s/
```

確認。

```bash
kubectl get deploy -n grpc-app
kubectl get pods -n grpc-app -o wide
kubectl get svc -n grpc-app
```

## Step 9: grpcurlで確認

port-forward。

```bash
kubectl port-forward svc/grpc-server 50051:50051 -n grpc-app
```

別ターミナル。

```bash
grpcurl -plaintext localhost:50051 list
```

ここまでで、

```text
grpc-server
↓
Docker image
↓
Deployment
↓
Pod x 3
↓
Service
↓
grpcurl
```

が完成

## Step 10: ArgoCDをインストール

```bash
kubectl apply -n argocd \
  -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
```

確認。

```bash
kubectl get pods -n argocd
kubectl get svc -n argocd
```

UIを見る。

```bash
kubectl port-forward svc/argocd-server -n argocd 8081:443
```

ブラウザ。

```text
https://localhost:8081
```

## Step 11: ArgoCD Applicationを作る

最終的にGitHubのmanifestをArgoCDに読ませます。

```text
GitHub
└── k8s/
    ├── deployment.yaml
    └── service.yaml
```

ArgoCDはそれを見て、

```text
GitHubの状態
  ↓
Kubernetesクラスタ
```

に同期

## 最終的に理解したいこと

この流れで以下が全部つながります。

```text
Cluster
  Kubernetes全体

Namespace
  grpc-app / argocd の分離

Node
  Podが配置される実行基盤

Deployment
  Pod数を管理する

ReplicaSet
  Deploymentの下でPod数を維持する

Pod
  grpc-serverコンテナが動く単位

Service
  grpc-server Pod群への固定入口

ArgoCD
  GitHubのmanifestとクラスタ状態を同期する
```

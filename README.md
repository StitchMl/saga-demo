# saga-demo

A demo implementation of the **SAGA** pattern in Go, with both **orchestration** and **choreography**, based on simple microservices (Order, Payment, Shipping), and an Event Bus. It provides:

- SAGA **orchestrated** with a centralised orchestrator
- SAGA **choreography** with Event Bus and event subscription/publication
- Local deployment with Docker Compose
- (Optional) Manifest Kubernetes
- Instructions for Deployment on AWS EC2.

---

## Prerequisiti

- Go ≥ 1.20
- Docker & Docker Compose ≥ v2
- Git
- (For EC2 deployment) AWS account and CLI configured.

---

## Struttura del progetto

```
saga-demo/
├── orchestrator/                   # Orchestrated SAGA
│   └── main.go
├── order-service/                  # Order Service (orchestration)
│   └── main.go
├── payment-service/                # Payment Service (orchestration)
│   └── main.go
├── shipping-service/               # Shipping Service (orchestration)
│   └── main.go
├── event-bus/                      # Event Bus (choreography)
│   └── main.go
├── order-service-choreo/           # Order Service (choreography)
│   └── main.go
├── payment-service-choreo/         # Payment Service (choreography)
│   └── main.go
├── shipping-service-choreo/        # Shipping Service (choreography)
│   └── main.go
├── frontend/
│   ├── index.html
│   ├── style.css
│   ├── app.js
│   ├── Dockerfile
│   └── nginx.conf
├── docker-compose.yml              # Local deployment
└── README.md                       # (this file)
```

---

## Local Setup with Docker Compose

1. Clone the repo:
   ```bash
   git clone https://github.com/stitchml/saga-demo.git
   cd saga-demo
   ```

2. Build and start all services:

   ```bash
   docker-compose up --build
   ````
3. Check the main services:

    * **Orchestrator**: `http://localhost:8080/saga`
    * **Event Bus**: `http://localhost:8070/publish`

The other services communicate internally via Compose's DNS (e.g. `order-service:8081`, `payment-service-choreo:8082`, etc.).

---

## Flow Testing

### Orchestrated flow

```bash
curl -X POST http://localhost:8080/saga \
  -H "Content-Type: application/json" \
  -d '{"id":"123","amount":50}'
```

### Choreographic flow

1. Submit an order:

   ```bash
   curl -X POST http://localhost:8081/orders \
     -H "Content-Type: application/json" \
     -d '{"OrderID":"123","Amount":50}'
   ```
2. Check the logs of `payment-service-choreo` and `shipping-service-choreo` for events.

---

## Deploy on AWS EC2

1. Start an EC2 Amazon Linux 2 t2.micro, open ports **22** (SSH) and **80** (HTTP) for your IP.
2. Connect:

   ```bash
   ssh -i key.pem ec2-user@<EC2_PUBLIC_IP>
   ```
3. Install Docker:

   ```bash
   sudo yum update -y
   sudo amazon-linux-extras install docker -y
   sudo service docker start
   sudo usermod -aG docker ec2-user
   ```
4. Clone and start:

   ```bash
   git clone https://github.com/tuo-username/saga-demo.git
   cd saga-demo
   docker-compose up -d --build
   ```
5. Browser Verification: `http://<EC2_PUBLIC_IP>:8080/saga`

---

## Kubernetes option (advanced)

1. Create Docker images and push on a registry (ECR/Docker Hub).
2. Apply manifest in `k8s/` (if present):

   ```bash
   kubectl apply -f k8s/
   ```
3. Check the status:

   ```bash
   kubectl get pods,svc
   ```

---

## Cleanup

To stop and remove local containers:

```bash
docker-compose down
```

To terminate services on EC2:

```bash
docker-compose down
exit
```

---

## Autori

* Matteo La Gioia
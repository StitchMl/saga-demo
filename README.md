# SAGA Pattern Project: Orchestration versus Choreography

This project implements a simple e-commerce app based on microservices to demonstrate and compare two approaches to the SAGA pattern: **Orchestration** and **Choreography**. The entire system is containerized using Docker and Docker Compose.

The backend is written in **Go**, while the frontend is a **React** app.

## Table of Contents

- [Architecture](#architecture)
  - [Choreographed Flow](#choreographed-flow)
  - [Orchestrated Flow](#orchestrated-flow)
  - [Common Services](#common-services)
- [Key Features](#key-features)
- [Project Requirements Compliance](#project-requirements-compliance)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Service Access](#service-access)
- [Project Structure](#project-structure)
- [Configurable Parameters](#configurable-parameters)
- [Testing](#testing)
  - [Manual Testing](#manual-testing)
  - [Automated Testing](#automated-testing)
- [EC2 Deployment](#ec2-deployment)

## Architecture

The system consists of a set of microservices that collaborate to manage e-commerce orders. Communication between services in the choreographed flow occurs via a message broker (**RabbitMQ**), while in the orchestrated flow, a central service manages it.

### Choreographed Flow

In this approach, there is no central coordinator. Services communicate by publishing events to RabbitMQ. Each service subscribes to events of interest and reacts accordingly, in turn publishing new events.

1.  The **Order Service** creates an order and publishes an `OrderCreated` event.
2.  The **Inventory Service** receives the event, reserves the products, and publishes `InventoryReserved`.
3.  The **Payment Service** receives the event, processes the payment, and publishes `PaymentProcessed` or `PaymentFailed`.
4.  In case of failure, compensating events (for example `RevertInventory`) are published to undo the previous operations.

### Orchestrated Flow

In this approach, the **Orchestrator** service manages the entire process synchronously.

1.  The API Gateway sends the order creation request to the **Orchestrator**.
2.  The Orchestrator sends direct commands (HTTP requests) to the various services in a predefined sequence:
    -   Create the order (Order Service).
    -   Reserve inventory (Inventory Service).
    -   Process payment (Payment Service).
3.  If a step fails, the Orchestrator is responsible for executing compensating operations by sending commands to undo the previous steps.

### Common Services

-   **API Gateway**: A single entry point for the frontend. It routes requests to the appropriate services based on the selected SAGA flow.
-   **Frontend**: A Single-Page App (SPA) built with React that allows users to interact with the system.
-   **Authentication Services**: Manage user registration and login (one for each flow).
-   **Order Services**: Manage the creation and status updates of orders.
-   **Inventory Services**: Manage product availability and stock reservation/release.
-   **Payment Services**: Simulate the payment process.
-   **RabbitMQ**: A message broker used for asynchronous, event-based communication in the choreographed flow.

## Key Features

-   **User Authentication**: Separate registration and login for the two flows.
-   **Product Catalog**: View available products with images and real-time availability.
-   **Order Creation**: Ability to create orders with one or more items.
-   **Dynamic Flow Selection**: Users can dynamically choose from the frontend whether to use the orchestrated or choreographed SAGA flow.
-   **Cross-Flow User Validation**: If a logged-in user switches flows, the system verifies their existence in the new flow and performs an automatic logout if they don’t exist.
-   **Failure Management**: The system correctly handles failures (for example, rejected payment, insufficient inventory) through SAGA compensating transactions.
-   **User Feedback**: The frontend provides immediate (or near-immediate via polling) feedback on the order status, even in the asynchronous flow.

## Project Requirements Compliance

-   **Language**: The backend is developed entirely in **Go**.
-   **Distributed System**: The architecture consists of 10+ Docker containers representing independent nodes.
-   **Configurable Parameters**: All critical parameters (service URLs, payment limits, connection strings) are managed via environment variables in the `docker-compose.yml` file, with no hard-coded values in the source code.
-   **Scalability and Fault Tolerance**:
    -   **Scalability**: `docker-compose` allows for horizontal scaling of individual services.
    -   **Fault Tolerance**: The SAGA pattern, and a message broker ensure data consistency even if a service failure. Compensating transactions roll back partial transactions.
-   **Shared State**: Dedicated services manage and update the state (inventory, orders) consistently through SAGA transactions.
-   **Multiple Clients**: The client-server architecture supports simultaneous access by multiple users.

## Prerequisites

-   [Docker](https://www.docker.com/get-started)
-   [Docker Compose](https://docs.docker.com/compose/install/)

## Getting Started

1.  Clone the repository.
2.  Open a terminal in the project root.
3.  Run the following command to build the images and start all containers:

    ```bash
    docker-compose up --build
    ```

## Service Access

Once the system is running, the services are accessible at the following addresses:

-   **Frontend Application**: [http://localhost:8090](http://localhost:8090)
-   **API Gateway**: `http://localhost:8000`
-   **RabbitMQ Management UI**: [http://localhost:15672](http://localhost:15672)
    -   **User**: `guest`
    -   **Password**: `guest`

## Project Structure

```
.
├── backend/
│   ├── choreographer_saga/ # Services for the choreographed flow
│   ├── orchestrator_saga/  # Services for the orchestrated flow
│   ├── common/             # Shared code (data store, types, etc.)
│   └── gateway/            # API Gateway code
├── frontend/               # React application code
├── scripts/                # Utility scripts (deployment, testing)
├── docker-compose.yml      # Configuration file for the entire architecture
└── README.md               # This file
```

## Configurable Parameters

The main environment variables can be modified in the `docker-compose.yml` file:

| Variable                           | Service                          | Description                                       |
|------------------------------------|----------------------------------|---------------------------------------------------|
| `GATEWAY_PORT`                     | api-gateway, frontend            | Exposed port for the API Gateway.                 |
| `RABBITMQ_URL`                     | All (choreographed backend)      | Connection URL for RabbitMQ.                      |
| `PAYMENT_AMOUNT_LIMIT`             | Payment Services                 | Amount limit to simulate failed payments.         |
| `..._SERVICE_URL`                  | Gateway, Orchestrator            | Internal URLs for inter-service communication.    |
| `RABBITMQ_PUBLISH_TIMEOUT_SECONDS` | All (choreographed backend)      | Timeout for publishing messages to RabbitMQ.      |

## Testing

### Manual Testing

The system was manually tested to cover all user flows and failure cases:

-   **Success Flow**: Creating a valid order that gets approved.
-   **Failure by Payment Limit**: Creating an order whose amount exceeds the configured limit.
-   **Failure by Random Error**: Simulating a network or payment gateway error.
-   **User Validation**: Testing the automatic logout on flow change if the user doesn’t exist in the new flow.

### Automated Testing

A test script is provided to automate the verification of the main flows.

```bash
# Example: test a successful order in the orchestrated flow
./scripts/test_saga.sh orchestrated success

# Example: test an order that fails due to the payment limit in the choreographed flow
./scripts/test_saga.sh choreographed fail_limit
```

The tests demonstrate that in each failure scenario, the compensating SAGA is executed correctly, restoring the system state (for example, inventory is released) and ensuring data consistency.

## EC2 Deployment

A script is provided to automate deployment to an Ubuntu-based EC2 instance.

```bash
# Usage: ./deploy_ec2.sh <user>@<host> /path/to/key.pem
./scripts/deploy_ec2.sh ubuntu@ec2-xx-xx-xx-xx.compute-1.amazonaws.com ~/.ssh/my-key.pem
```
The script handles:
1. Verifying SSH connectivity.
2. Installing Docker, Docker Compose, and Git.
3. Cloning the repository.
4. Starting the app with `docker-compose`.


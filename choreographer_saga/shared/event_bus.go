package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/StitchMl/saga-demo/common/events"
	"log"
	"net"
	"syscall"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

// EventHandler is a type of function that handles events.
type EventHandler func(event interface{})

// EventBus is an event bus based on RabbitMQ.
type EventBus struct {
	conn        *amqp091.Connection
	channel     *amqp091.Channel
	exchange    string
	subscribers map[events.EventType][]EventHandler // Maintains local handlers for dispatch
}

// NewEventBus creates a new instance of EventBus and connects to RabbitMQ.
// The exchange 'saga_events' will be used for event routing.
func NewEventBus(rabbitMQURL string) (*EventBus, error) {
	// Create a custom dialer to control the network connection.
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,  // Timeout for connection
		KeepAlive: 30 * time.Second, // Interval for keep-alive messages

		// Control function to ensure use IPv4 for RabbitMQ connections.
		Control: func(network, address string, c syscall.RawConn) error {
			// If the network is TCP, enforce using IPv4.
			if network == "tcp" {
				network = "tcp4"
			}
			return nil
		},
	}

	// Create a dial function that uses the custom dialer.
	amqpDialFunc := func(network, addr string) (net.Conn, error) {
		return dialer.DialContext(context.Background(), network, addr)
	}

	// Now use DialConfig to pass the customized dialer.
	// The DialConfig function allows configuring advanced connection options.
	conn, err := amqp091.DialConfig(rabbitMQURL, amqp091.Config{
		Dial: amqpDialFunc, // Use the adapter function here
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		if err := conn.Close(); err != nil {
			log.Printf("[EventBus] Failed to close connection after channel error: %v", err)
		}
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	exchangeName := "saga_events"
	err = channel.ExchangeDeclare(
		exchangeName, // name
		"topic",      // type (topic for flexible routing)
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		if err := channel.Close(); err != nil {
			log.Printf("[EventBus] Failed to close channel after exchange declaration error: %v", err)
		}
		if err := conn.Close(); err != nil {
			log.Printf("[EventBus] Failed to close connection after exchange declaration error: %v", err)
		}
		return nil, fmt.Errorf("failed to declare an exchange: %w", err)
	}

	log.Printf("[EventBus] Connected to RabbitMQ at %s. Exchange '%s' declared.", rabbitMQURL, exchangeName)

	return &EventBus{
		conn:        conn,
		channel:     channel,
		exchange:    exchangeName,
		subscribers: make(map[events.EventType][]EventHandler),
	}, nil
}

// Close closes the connection and the RabbitMQ channel.
func (eb *EventBus) Close() {
	if eb.channel != nil {
		if err := eb.channel.Close(); err != nil {
			log.Printf("[EventBus] Failed to close channel: %v", err)
		} else {
			log.Println("[EventBus] RabbitMQ channel closed.")
		}
	}
	if eb.conn != nil {
		if err := eb.conn.Close(); err != nil {
			log.Printf("[EventBus] Failed to close connection: %v", err)
		} else {
			log.Println("[EventBus] RabbitMQ connection closed.")
		}
	}
	log.Println("[EventBus] RabbitMQ connection closed.")
}

// Publish publishes an event on RabbitMQ.
func (eb *EventBus) Publish(event events.GenericEvent) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// The routing key is the type of event, allowing subscribers to filter.
	routingKey := string(event.Type)

	err = eb.channel.PublishWithContext(
		ctx,
		eb.exchange, // exchange
		routingKey,  // routing key (event type)
		false,       // mandatory
		false,       // immediate
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish message: %w", err)
	}

	log.Printf("[EventBus] Published event '%s' for Order %s to exchange '%s' with routing key '%s'", event.Type, event.OrderID, eb.exchange, routingKey)
	return nil
}

// Subscribe registers an EventHandler for a given EventType and starts consuming messages from RabbitMQ.
func (eb *EventBus) Subscribe(eventType events.EventType, handler EventHandler) error {
	// Register the handler locally
	eb.subscribers[eventType] = append(eb.subscribers[eventType], handler)

	// Declare a temporary and anonymous queue for this service
	q, err := eb.channel.QueueDeclare(
		"",    // name—generates a random queue name
		false, // durable - the queue is deleted on disconnection
		true,  // autoDelete - the queue is eliminated when it has no more consumers.
		false, // exclusive—the queue is private for this connection
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	// Bind the queue to the exchange using the event type as a routing key.
	err = eb.channel.QueueBind(
		q.Name,            // queue name
		string(eventType), // routing key (event type)
		eb.exchange,       // exchange
		false,             // noWait
		nil,               // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to bind a queue: %w", err)
	}

	messages, err := eb.channel.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto ack—Automatic ACK of messages
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	log.Printf("[EventBus] Subscribed to event type: %s. Consuming from queue '%s'.", eventType, q.Name)

	go func() {
		for d := range messages {
			var genericEvent events.GenericEvent
			err := json.Unmarshal(d.Body, &genericEvent)
			if err != nil {
				log.Printf("[EventBus] Error unmarshalling message: %v", err)
				continue
			}

			// Find and dispatch the payload to the local registered handler
			if genericEvent.Type == eventType { // Ensure that the event received matches the subscribed type
				log.Printf("[EventBus] Received message for type %s: Order %s", genericEvent.Type, genericEvent.OrderID)
				handler(genericEvent.Payload) // Pass the specific payload to the handler
			}
		}
	}()

	return nil
}

package shared

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	events "github.com/StitchMl/saga-demo/common/types"
	amqp "github.com/rabbitmq/amqp091-go"
)

// EventHandler is a type of function that handles events.
type EventHandler func(event interface{})

// EventBus is an event bus based on RabbitMQ.
type EventBus struct {
	conn        *amqp.Connection
	channel     *amqp.Channel
	exchange    string
	subscribers map[events.EventType][]EventHandler
}

// NewEventBus creates a new instance of EventBus and connects to RabbitMQ.
func NewEventBus(rabbitMQURL string) (*EventBus, error) {
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return nil, fmt.Errorf("RabbitMQ connection failed: %w", err)
	}
	channel, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}

	const exchangeName = "saga_events"
	if err := channel.ExchangeDeclare(
		exchangeName, "topic", true, false, false, false, nil,
	); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("exchange statement failed: %w", err)
	}

	log.Printf("[EventBus] Connected to RabbitMQ %s. Exchange '%s' declared.", rabbitMQURL, exchangeName)

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
		}
	}
	if eb.conn != nil {
		if err := eb.conn.Close(); err != nil {
			log.Printf("[EventBus] Failed to close connection: %v", err)
		}
	}
}

// Publish publishes an event on RabbitMQ.
func (eb *EventBus) Publish(event events.GenericEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := eb.channel.PublishWithContext(
		ctx,
		eb.exchange,
		string(event.Type),
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		}); err != nil {
		return fmt.Errorf("publish message: %w", err)
	}
	log.Printf("[EventBus] Published event '%s' for Order %s", event.Type, event.OrderID)
	return nil
}

// Subscribe registers an EventHandler for a given EventType and starts consuming messages from RabbitMQ.
func (eb *EventBus) Subscribe(eventType events.EventType, handler EventHandler) error {
	q, err := eb.channel.QueueDeclare("", false, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("queue declare: %w", err)
	}
	if err := eb.channel.QueueBind(q.Name, string(eventType), eb.exchange, false, nil); err != nil {
		return fmt.Errorf("queue bind: %w", err)
	}
	messages, err := eb.channel.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	go func() {
		for d := range messages {
			var e events.GenericEvent
			if json.Unmarshal(d.Body, &e) == nil && e.Type == eventType {
				handler(e)
			}
		}
	}()
	return nil
}

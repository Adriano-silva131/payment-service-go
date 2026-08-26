package kafka

// OrderEventsTopic is the topic order-service publishes order.created.v1 onto — same
// topic name the old Java payment-service consumed from.
const OrderEventsTopic = "order-events"

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/ovh/go-ovh/ovh"
)

func main() {
	// Retrieve OVH API credentials from environment variables
	endpoint := os.Getenv("OVH_ENDPOINT")
	appKey := os.Getenv("OVH_APPLICATION_KEY")
	appSecret := os.Getenv("OVH_APPLICATION_SECRET")
	consumerKey := os.Getenv("OVH_CONSUMER_KEY")

	if endpoint == "" || appKey == "" || appSecret == "" || consumerKey == "" {
		log.Fatalf("Please set OVH_ENDPOINT, OVH_APPLICATION_KEY, OVH_APPLICATION_SECRET, and OVH_CONSUMER_KEY environment variables")
	}

	// Create an OVH client
	client, err := ovh.NewClient(
		endpoint,
		appKey,
		appSecret,
		consumerKey,
	)
	if err != nil {
		log.Fatalf("Error creating OVH client: %v", err)
	}

	// Step 1: Create a new cart
	cart := make(map[string]interface{})
	expireDate := time.Now().AddDate(0, 1, 0).Format(time.RFC3339)
	err = client.Post("/order/cart", map[string]interface{}{
		"ovhSubsidiary": "US",
		"description":   "Automated Dedicated Server Order",
		"expire":        expireDate,
	}, &cart)
	if err != nil {
		log.Fatalf("Error creating cart: %v", err)
	}
	cartID := cart["cartId"].(string)
	fmt.Printf("Created Cart with ID: %s\n", cartID)

	// Step 2: Assign the cart to the logged-in user
	err = client.Post("/order/cart/"+cartID+"/assign", nil, nil)
	if err != nil {
		log.Fatalf("Error assigning cart: %v", err)
	}
	fmt.Println("Assigned cart to the logged-in user.")

	// Step 3: Add a dedicated server to the cart (using planCode "24rise01-us")
	server := make(map[string]interface{})
	err = client.Post("/order/cart/"+cartID+"/baremetalServers", map[string]interface{}{
		"duration":    "P1M",
		"planCode":    "24rise01-us",
		"pricingMode": "default",
		"quantity":    1,
	}, &server)
	if err != nil {
		log.Fatalf("Error adding server to cart: %v", err)
	}

	// Convert itemId from json.Number to string then to int64
	itemIDStr := server["itemId"].(json.Number).String() // Convert json.Number to string
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)   // Parse string to int64
	if err != nil {
		log.Fatalf("Error converting itemId to int64: %v", err)
	}
	fmt.Printf("Added Server to Cart with Item ID: %d\n", itemID)

	// Step 4: Configure the server (dedicated_os, region, dedicated_datacenter)
	configItems := []struct {
		Label string
		Value string
	}{
		{"dedicated_os", "none_64_en"},
		{"region", "united_states"},
		{"dedicated_datacenter", "hil"},
	}

	for _, config := range configItems {
		configResponse := make(map[string]interface{})
		err = client.Post(fmt.Sprintf("/order/cart/%s/item/%d/configuration", cartID, itemID), map[string]interface{}{
			"label": config.Label,
			"value": config.Value,
		}, &configResponse)
		if err != nil {
			log.Fatalf("Error configuring %s: %v", config.Label, err)
		}
		fmt.Printf("Configured %s with value %s\n", config.Label, config.Value)
	}

	// Step 5: Add an option (e.g., vrack bandwidth)
	optionResponse := make(map[string]interface{})
	err = client.Post(fmt.Sprintf("/order/cart/%s/baremetalServers/options", cartID), map[string]interface{}{
		"duration":    "P1M",
		"itemId":      itemID,
		"planCode":    "vrack-bandwidth-1000-24rise-us", // Example option
		"pricingMode": "default",
		"quantity":    1,
	}, &optionResponse)
	if err != nil {
		log.Fatalf("Error adding option: %v", err)
	}
	fmt.Println("Added option to the server.")

	// Step 6: Validate the order and proceed to checkout
	order := make(map[string]interface{})
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = client.Post(fmt.Sprintf("/order/cart/%s/checkout", cartID), nil, &order)
		if err == nil {
			break // Exit the loop if the call succeeds
		}
		log.Printf("Attempt %d failed with error: %v. Retrying...\n", attempt, err)
		time.Sleep(time.Duration(math.Pow(2, float64(attempt))) * time.Second) // Exponential backoff
	}

	if err != nil {
		log.Fatalf("Order validation failed: failed after %d retries", maxRetries)
	}
	orderID := fmt.Sprintf("%v", order["orderId"])
	fmt.Printf("Order validated. Order ID: %s\n", orderID)

	// Step 7: Fetch available payment methods for this order
	var paymentMethods []map[string]interface{}
	err = client.Get(fmt.Sprintf("/me/order/%s/availablePaymentMethod", orderID), &paymentMethods)
	if err != nil {
		log.Fatalf("Error fetching payment methods: %v", err)
	}
	fmt.Printf("Available Payment Methods: %v\n", paymentMethods)

	// Example: Use the first payment method to complete the order
	if len(paymentMethods) > 0 {
		paymentID := paymentMethods[0]["id"].(json.Number)
		paymentType := paymentMethods[0]["type"].(string)

		// Step 8: Pay for the order
		paymentResponse := make(map[string]interface{})
		err = client.Post(fmt.Sprintf("/me/order/%s/pay", orderID), map[string]interface{}{
			"paymentMethod": map[string]interface{}{
				"id":   paymentID,
				"type": paymentType,
			},
		}, &paymentResponse)
		if err != nil {
			log.Fatalf("Error paying for the order: %v", err)
		}
		fmt.Println("Order has been successfully paid.")
	} else {
		log.Fatal("No available payment methods found.")
	}
}

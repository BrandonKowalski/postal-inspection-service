package classifier

import (
	"strings"
)

// IsTransactional checks if an email subject indicates a transactional email
// (order confirmations, shipping updates, receipts, etc.) vs marketing
func IsTransactional(subject string) bool {
	lower := strings.ToLower(subject)

	// Transactional indicators - things you want to receive
	transactionalKeywords := []string{
		// Order related
		"order confirm",
		"your order",
		"order #",
		"order number",
		"order placed",
		"order received",
		"order status",
		"order update",

		// Shipping related
		"shipped",
		"shipping confirm",
		"shipping update",
		"delivery confirm",
		"delivery update",
		"out for delivery",
		"delivered",
		"tracking",
		"in transit",
		"package",
		"shipment",

		// Receipt/Invoice related
		"receipt",
		"invoice",
		"payment confirm",
		"payment received",
		"transaction",
		"purchase confirm",

		// Account related (important notifications)
		"password reset",
		"verify your",
		"verification",
		"security alert",
		"login attempt",
		"account confirm",
		"subscription confirm",

		// Booking/Reservation related
		"booking confirm",
		"reservation confirm",
		"itinerary",
		"appointment",
		"ticket",

		// Refund/Return related
		"refund",
		"return confirm",
		"return label",
		"exchange",
	}

	for _, keyword := range transactionalKeywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}

	// Marketing indicators - things you don't want
	marketingKeywords := []string{
		// Sales/Promotions
		"% off",
		"sale",
		"deal",
		"discount",
		"save $",
		"save up to",
		"limited time",
		"flash sale",
		"clearance",
		"black friday",
		"cyber monday",
		"holiday",
		"special offer",
		"exclusive offer",
		"promo",
		"coupon",

		// Newsletter/Marketing
		"newsletter",
		"weekly",
		"monthly",
		"digest",
		"roundup",
		"what's new",
		"new arrivals",
		"just dropped",
		"trending",
		"top picks",
		"recommended for you",
		"you might like",
		"based on your",

		// Engagement bait
		"don't miss",
		"last chance",
		"ending soon",
		"act now",
		"hurry",
		"only hours left",
		"reminder:",
		"we miss you",
		"come back",

		// Generic marketing
		"shop now",
		"buy now",
		"free shipping",
		"new collection",
		"introducing",
		"check out",
		"discover",
		"explore",
	}

	for _, keyword := range marketingKeywords {
		if strings.Contains(lower, keyword) {
			return false // Explicitly marketing
		}
	}

	// Default: if we can't classify, assume marketing (safer to delete)
	return false
}

// ClassifyEmail returns a classification result with reasoning
type Classification struct {
	IsTransactional bool
	Reason          string
}

func Classify(subject string) Classification {
	lower := strings.ToLower(subject)

	// Check transactional first
	transactionalPatterns := map[string]string{
		"order confirm":        "Order confirmation",
		"your order":           "Order notification",
		"shipped":              "Shipping notification",
		"delivery":             "Delivery update",
		"tracking":             "Tracking update",
		"receipt":              "Receipt",
		"invoice":              "Invoice",
		"payment":              "Payment notification",
		"password reset":       "Security/Account",
		"verification":         "Account verification",
		"booking confirm":      "Booking confirmation",
		"reservation":          "Reservation",
		"refund":               "Refund notification",
		"return":               "Return notification",
		"appointment":          "Appointment",
		"itinerary":            "Travel itinerary",
		"subscription confirm": "Subscription confirmation",
	}

	for pattern, reason := range transactionalPatterns {
		if strings.Contains(lower, pattern) {
			return Classification{IsTransactional: true, Reason: reason}
		}
	}

	// Check marketing
	marketingPatterns := map[string]string{
		"% off":        "Discount promotion",
		"sale":         "Sale promotion",
		"deal":         "Deal promotion",
		"newsletter":   "Newsletter",
		"don't miss":   "Marketing urgency",
		"last chance":  "Marketing urgency",
		"shop now":     "Marketing CTA",
		"new arrivals": "Product marketing",
		"we miss you":  "Re-engagement",
		"recommended":  "Recommendation marketing",
	}

	for pattern, reason := range marketingPatterns {
		if strings.Contains(lower, pattern) {
			return Classification{IsTransactional: false, Reason: reason}
		}
	}

	return Classification{IsTransactional: false, Reason: "Unknown/Default to marketing"}
}

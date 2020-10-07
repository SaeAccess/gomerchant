// Package stripe implements GoMerchant payment gateway for Stripe.
package stripe

import (
	"fmt"
	"time"

	"github.com/qor/gomerchant"
	stripe "github.com/stripe/stripe-go"
	"github.com/stripe/stripe-go/charge"
	"github.com/stripe/stripe-go/refund"
	"github.com/stripe/stripe-go/token"
)

// Stripe implements gomerchant.PaymetGateway interface.
type Stripe struct {
	Config *Config
}

var _ gomerchant.PaymentGateway = &Stripe{}

// Config stripe config
type Config struct {
	Key string
}

// New creates Stripe struct.
func New(config *Config) *Stripe {
	stripe.Key = config.Key

	return &Stripe{
		Config: config,
	}
}

//var capture bool = false

// Authorize uses strips api to authorize payment against the supplied credit card
func (*Stripe) Authorize(amount uint64, params gomerchant.AuthorizeParams) (gomerchant.AuthorizeResponse, error) {
	// TODO fix me - Stripe uses cents for USD therefore need to multiply by 100 if currency is USD
	// Hard code for now with assumption amount is always USD
	amount *= 100
	int64Amount := int64(amount)

	chargeParams := &stripe.ChargeParams{
		Amount:      &int64Amount,
		Currency:    &params.Currency,
		Description: &params.Description,
		//		Capture:     &capture,
	}

	chargeParams.AddMetadata("order_id", params.OrderID)

	if params.PaymentMethod != nil {
		if params.PaymentMethod.CreditCard != nil {
			t, err := toStripeToken(params.Customer, params.PaymentMethod.CreditCard, params.BillingAddress)
			if err != nil {
				return gomerchant.AuthorizeResponse{}, err
			}

			source := stripe.SourceParams{
				Token: &t.ID,
			}

			chargeParams.Source = &source
		}

		if params.PaymentMethod.SavedCreditCard != nil {
			if len(params.PaymentMethod.SavedCreditCard.CustomerID) > 0 {
				chargeParams.Customer = &params.PaymentMethod.SavedCreditCard.CustomerID
			}

			chargeParams.SetSource(params.PaymentMethod.SavedCreditCard.CreditCardID)
		}
	}

	charge, err := charge.New(chargeParams)
	if charge != nil {
		return gomerchant.AuthorizeResponse{TransactionID: charge.ID}, err
	}

	return gomerchant.AuthorizeResponse{}, err
}

// CompleteAuthorize completes the authorization for a charge
func (*Stripe) CompleteAuthorize(paymentID string, params gomerchant.CompleteAuthorizeParams) (gomerchant.CompleteAuthorizeResponse, error) {
	return gomerchant.CompleteAuthorizeResponse{}, nil
}

// Capture performs the settlement process for a charge (issuer to merchant)
func (*Stripe) Capture(transactionID string, params gomerchant.CaptureParams) (gomerchant.CaptureResponse, error) {
	_, err := charge.Capture(transactionID, nil)
	return gomerchant.CaptureResponse{TransactionID: transactionID}, err
}

// Refund refunds a prior charge transaction
func (s *Stripe) Refund(transactionID string, amount uint, params gomerchant.RefundParams) (gomerchant.RefundResponse, error) {
	transaction, err := s.Query(transactionID)

	// TODO fix me - Stripe uses cents for USD therefore need to multiply by 100 if currency is USD
	// Hard code for now with assumption amount is always USD
	amount *= 100

	if err == nil {
		if transaction.Captured {
			int64Amount := int64(amount)
			_, err = refund.New(&stripe.RefundParams{
				Charge: &transactionID,
				Amount: &int64Amount,
			})
		} else {
			int64Amount := int64(transaction.Amount - int(amount))
			_, err = charge.Capture(transactionID, &stripe.CaptureParams{
				Amount: &int64Amount,
			})
		}
	}

	return gomerchant.RefundResponse{TransactionID: transactionID}, err
}

// Void voids a prior charge transaction
func (*Stripe) Void(transactionID string, params gomerchant.VoidParams) (gomerchant.VoidResponse, error) {
	refundParams := &stripe.RefundParams{
		Charge: &transactionID,
	}
	_, err := refund.New(refundParams)
	return gomerchant.VoidResponse{TransactionID: transactionID}, err
}

// Query retrieves a charge transaction
func (*Stripe) Query(transactionID string) (gomerchant.Transaction, error) {
	c, err := charge.Get(transactionID, nil)
	created := time.Unix(c.Created, 0)
	transaction := gomerchant.Transaction{
		ID:        c.ID,
		Amount:    int(c.Amount - c.AmountRefunded),
		Currency:  string(c.Currency),
		Captured:  c.Captured,
		Paid:      c.Paid,
		Cancelled: c.Refunded,
		Status:    c.Status,
		CreatedAt: &created,
	}

	if transaction.Cancelled {
		transaction.Paid = false
		transaction.Captured = false
	}

	return transaction, err
}

// toStripeToken creates a stripe token for the supplied charge
func toStripeToken(customer string, cc *gomerchant.CreditCard, billingAddress *gomerchant.Address) (*stripe.Token, error) {
	var (
		expMonth = fmt.Sprint(cc.ExpMonth)
		expYear  = fmt.Sprint(cc.ExpYear)
	)

	cp := &stripe.CardParams{
		Name:     &cc.Name,
		Number:   &cc.Number,
		ExpMonth: &expMonth,
		ExpYear:  &expYear,
		CVC:      &cc.CVC,
	}

	// Add customer if specified
	addCustomer := len(customer) > 0
	if addCustomer {
		cp.Customer = &customer
	}

	if billingAddress != nil {
		cp.AddressLine1 = &billingAddress.Address1
		cp.AddressLine1 = &billingAddress.Address2
		cp.AddressCity = &billingAddress.City
		cp.AddressState = &billingAddress.State
		cp.AddressZip = &billingAddress.ZIP
		cp.AddressCountry = &billingAddress.Country
	}

	params := &stripe.TokenParams{
		Card: cp,
	}

	if addCustomer {
		params.Customer = &customer
	}

	return token.New(params)
}

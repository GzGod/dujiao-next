package orderwiring

import (
	"testing"

	"github.com/dujiao-next/internal/constants"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type paymentListStub []paymentdomain.Payment

func (s paymentListStub) ListByOrderID(uint) ([]paymentdomain.Payment, error) { return s, nil }

func TestPaymentProofUsesSuccessfulX402Settlement(t *testing.T) {
	settlement := jsonmap.JSON{"x402_settlement": map[string]interface{}{
		"transaction": "0xabc", "network": "eip155:84532", "payer": "0x01",
	}}
	payments := paymentListStub{
		{ProviderType: constants.PaymentProviderX402, Status: constants.PaymentStatusExpired, ProviderPayload: jsonmap.JSON{"x402_settlement": map[string]interface{}{"transaction": "0xold"}}},
		{ProviderType: constants.PaymentProviderWallet, Status: constants.PaymentStatusSuccess},
		{ProviderType: constants.PaymentProviderX402, Status: constants.PaymentStatusSuccess, ProviderPayload: settlement},
	}
	proof, err := orderPaymentProofAdapter{payments: payments}.GetPaymentProof(1)
	if err != nil {
		t.Fatal(err)
	}
	if proof == nil || proof.Transaction != "0xabc" || proof.Network != "eip155:84532" || proof.Payer != "0x01" {
		t.Fatalf("unexpected proof: %+v", proof)
	}
}

func TestPaymentProofEmptyWithoutX402(t *testing.T) {
	proof, err := orderPaymentProofAdapter{payments: paymentListStub{
		{ProviderType: constants.PaymentProviderWallet, Status: constants.PaymentStatusSuccess},
	}}.GetPaymentProof(1)
	if err != nil || proof != nil {
		t.Fatalf("expected no proof, got %+v %v", proof, err)
	}
}
